// Package webhooks manda el payload de una alerta a los destinos
// registrados (ver docs/agente-api.md, sección Webhooks): firmado con
// HMAC-SHA256 si el webhook tiene secreto, con reintentos y sin seguir
// redirecciones. No conoce agent/store ni agent/alerts — Alert/Device en
// Payload son `any` a propósito: este paquete solo sabe serializar y
// firmar lo que le pasan, así cmd/vigia-agent puede mandarlo desde una
// goroutine propia sin bloquear el escaneo (ver "envío asíncrono" en el
// contrato) sin que este paquete tenga que saber nada del dominio.
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// DefaultTimeout: por intento (ver docs/agente-api.md: "Timeout 5 s").
const DefaultTimeout = 5 * time.Second

// DefaultBackoff: espera antes de cada reintento (ver docs/agente-api.md:
// "hasta 3 intentos con espera (1 s, 5 s, 25 s)"). El contrato es
// ambiguo sobre si son 3 intentos EN TOTAL o 3 REINTENTOS tras el primer
// envío: con solo 3 intentos totales habría apenas dos esperas posibles
// (antes del 2.º y del 3.º), y el tercer valor (25 s) nunca se usaría.
// Elegimos la lectura que no desperdicia ninguno de los tres números:
// el primer envío es inmediato, y estos tres valores son la espera antes
// de cada uno de hasta 3 reintentos — 4 POST como máximo por entrega. Ver
// la entrega de la tarea para más detalle de esta decisión.
var DefaultBackoff = []time.Duration{1 * time.Second, 5 * time.Second, 25 * time.Second}

// AgentInfo es el "agent" del payload (ver docs/agente-api.md).
type AgentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Payload es el cuerpo JSON que se manda al webhook (ver
// docs/agente-api.md: `{"event": "alert", "alert": Alert, "device":
// KnownDevice|null, "agent": {"id","name"}}`). Alert/Device son `any` a
// propósito — ver el comentario del paquete.
type Payload struct {
	Event  string    `json:"event"`
	Alert  any       `json:"alert"`
	Device any       `json:"device"`
	Agent  AgentInfo `json:"agent"`
}

// Target es un webhook registrado, listo para mandarle algo (ver
// store.Webhook, de donde sale).
type Target struct {
	ID     int64
	URL    string
	Secret string
}

// Result es el desenlace de una entrega (para persistir
// lastStatus/lastError, ver store.UpdateWebhookResult). Status es nil si
// la petición nunca se completó (red, timeout, URL inválida); si el
// servidor remoto respondió algo, Status trae ese código sin importar si
// fue 2xx — Err va aparte, solo para fallos de transporte.
type Result struct {
	Status *int
	Err    error
}

// Success: hubo una respuesta 2xx.
func (r Result) Success() bool { return r.Status != nil && *r.Status >= 200 && *r.Status < 300 }

// ErrBlockedAddress se devuelve cuando un webhook intenta conectar a una IP
// bloqueada (loopback, unspecified o link-local/metadata).
var ErrBlockedAddress = errors.New("webhooks: destino bloqueado (loopback/unspecified/link-local)")

// Sender manda payloads firmados a un Target. El zero-value ya funciona
// (usa los valores por defecto del paquete); los campos son overridables
// para pruebas (httptest, backoffs cortos).
type Sender struct {
	// Client: inyectable para pruebas. nil construye uno con Timeout(),
	// Control anti-SSRF y sin seguir redirecciones (ver docs/agente-api.md:
	// "el agente nunca sigue redirecciones").
	Client *http.Client
	// Version va en el User-Agent: "Vigia-Agent/<versión>". "" usa "dev".
	Version string
	// Timeout por intento. <= 0 usa DefaultTimeout.
	Timeout time.Duration
	// Backoff antes de cada reintento. nil usa DefaultBackoff.
	Backoff []time.Duration

	// Solo para tests unitarios dentro del paquete contra httptest en loopback.
	allowLoopbackForTest bool
}

// NewSender crea un Sender listo para producción: Timeout/Backoff del
// contrato y un *http.Client propio con protección anti-SSRF que nunca
// sigue redirecciones.
func NewSender(version string) *Sender {
	s := &Sender{
		Version: version,
		Timeout: DefaultTimeout,
		Backoff: DefaultBackoff,
	}
	s.Client = s.newHTTPClient(false)
	return s
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func (s *Sender) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return s.newHTTPClient(true)
}

func (s *Sender) newHTTPClient(disableKeepAlives bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   s.timeout(),
		KeepAlive: 30 * time.Second,
		Control:   s.dialControl,
	}
	return &http.Client{
		Timeout:       s.timeout(),
		CheckRedirect: noRedirect,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         dialer.DialContext,
			DisableKeepAlives:   disableKeepAlives,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: s.timeout(),
		},
	}
}

func (s *Sender) dialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ErrBlockedAddress
	}
	if s.allowLoopbackForTest && ip.IsLoopback() {
		return nil
	}
	if isBlockedIP(ip) {
		return ErrBlockedAddress
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func (s *Sender) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return DefaultTimeout
}

func (s *Sender) backoff() []time.Duration {
	if s.Backoff != nil {
		return s.Backoff
	}
	return DefaultBackoff
}

// Attempt manda body UNA sola vez a target (sin reintentos: los da
// Deliver, llamando a Attempt una vez por intento; y lo usan
// POST /api/v1/webhooks/{id}/test y `vigia-agent webhook test`, que
// quieren un solo intento inmediato). Valida que la URL sea http/https
// antes de intentar nada — "Solo http/https" del contrato.
func (s *Sender) Attempt(ctx context.Context, target Target, body []byte, eventType, deliveryID string) (int, error) {
	if err := ValidateWebhookURL(target.URL); err != nil {
		return 0, fmt.Errorf("webhooks: url inválida %q: %w", target.URL, err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	version := s.Version
	if version == "" {
		version = "dev"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Vigia-Agent/"+version)
	req.Header.Set("X-Vigia-Event", eventType)
	req.Header.Set("X-Vigia-Delivery", deliveryID)
	if target.Secret != "" {
		req.Header.Set("X-Vigia-Signature", "sha256="+Sign(target.Secret, body))
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10)) // liberar la conexión para el pool
	return resp.StatusCode, nil
}

// Deliver manda body a target con reintentos (ver backoff()): hasta
// len(backoff)+1 intentos en total, parando en el primer 2xx. Cada
// intento comparte el mismo X-Vigia-Delivery (identifica la entrega, no
// el intento) — ver newDeliveryID.
func (s *Sender) Deliver(ctx context.Context, target Target, body []byte, eventType string) Result {
	deliveryID, err := NewDeliveryID()
	if err != nil {
		return Result{Err: err}
	}

	backoff := s.backoff()
	var last Result
	for attempt := 0; ; attempt++ {
		status, err := s.Attempt(ctx, target, body, eventType, deliveryID)
		if err != nil {
			last = Result{Err: err}
		} else {
			last = Result{Status: &status}
		}
		if last.Success() || attempt >= len(backoff) {
			return last
		}
		select {
		case <-ctx.Done():
			return last
		case <-time.After(backoff[attempt]):
		}
	}
}

// ValidateWebhookURL comprueba que raw sea una URL http o https válida,
// con host no vacío y sin credenciales (userinfo).
func ValidateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("formato de url inválido")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return errors.New("debe ser http o https")
	}
	if u.User != nil {
		return errors.New("no debe incluir credenciales")
	}
	if u.Hostname() == "" {
		return errors.New("falta el host")
	}
	return nil
}

// Sign calcula HMAC-SHA256(secret, body) en hex — lo que lleva
// X-Vigia-Signature tras "sha256=" (ver docs/agente-api.md). Exportada
// para que quien reciba el webhook (y los tests) puedan verificarla con
// el mismo cálculo.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// NewDeliveryID genera un UUID v4 para X-Vigia-Delivery, con crypto/rand
// a mano en vez de github.com/google/uuid: mismo motivo que
// agent.newUUIDv4 (ver agent/identity.go) — no se suma una dependencia
// propia solo para 16 bytes aleatorios con dos nibbles fijos.
func NewDeliveryID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("delivery id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // versión 4
	b[8] = (b[8] & 0x3f) | 0x80 // variante RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
