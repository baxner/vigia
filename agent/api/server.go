package api

import (
	"net/http"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/internet"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/webhooks"
)

// scanTrigger es lo que POST /api/v1/scan y GET /api/v1/info necesitan del
// scheduler. *scheduler.Scheduler lo cumple tal cual; la interfaz evita que
// este paquete dependa de scheduler solo para poder probar el 202/409 sin
// carreras de tiempo reales.
type scanTrigger interface {
	TriggerNow() bool
	LastScanAt() time.Time
	Interval() time.Duration
}

// internetProvider es lo que GET /api/v1/internet y POST
// /api/v1/internet/speedtest necesitan del medidor en vivo (ver
// cmd/vigia-agent/internet.go, que lo implementa: internetMonitor +
// speedTestRunner). El store solo tiene las ventanas YA cerradas, las
// caídas y las pruebas de velocidad históricas (ver agent/store/internet.go);
// el estado AHORA (state/since/now/ipv6) y el 409 de una prueba en curso
// viven en memoria, igual que scanTrigger con el scheduler.
type internetProvider interface {
	// InternetNow da el estado en vivo: el último resultado evaluado, desde
	// cuándo, la última Sample medida y el estado de IPv6.
	InternetNow() (state internet.State, since time.Time, now internet.Sample, ipv6 internet.IPv6Status)
	// TriggerSpeedTest arranca una prueba: false si ya hay una corriendo
	// (ver POST /api/v1/internet/speedtest, "409 si ya hay una").
	TriggerSpeedTest() bool
}

// Deps son las dependencias del servidor. Store es el único campo
// obligatorio en el sentido estricto; sin Hub, el stream simplemente nunca
// tiene nada que mandar (no es un error armar un Server sin él, pero
// cmd/vigia-agent siempre pasa el mismo Hub que alimenta el scheduler).
type Deps struct {
	Store     *store.Store
	Scheduler scanTrigger
	Identity  agent.Identity
	Version   string
	Hub       *Hub
	// Warning da el aviso de salud del último escaneo ("" = ninguno). Opcional.
	Warning func() string
	// Addresses da las IPv4 de las interfaces activas (ver GET
	// /api/v1/info, "addresses" en docs/agente-api.md). nil usa
	// agent.LocalIPv4Addresses.
	Addresses func() []string
	// Sender manda las entregas de prueba de POST
	// /api/v1/webhooks/{id}/test. nil usa webhooks.NewSender(Version).
	Sender *webhooks.Sender
	// Internet es el medidor en vivo de "Internet (v1.4)" (ver
	// internetProvider). nil: los 3 endpoints que lo necesitan (GET
	// /api/v1/internet y POST .../speedtest) responden 503 — las pruebas
	// que no tocan Internet no tienen por qué armar uno.
	Internet internetProvider
	// CertSHA256 es la huella SHA-256 (hex) del certificado TLS propio del
	// agente (ver TLS v1.5, docs/agente-api.md: "certSha256" en /info y
	// /pair; cmd/vigia-agent la saca de agent.LoadOrCreateCert). "" en las
	// pruebas que no montan TLS: el campo sale vacío, no se omite.
	CertSHA256 string
}

// Server implementa el contrato HTTP del agente (docs/agente-api.md).
type Server struct {
	store       *store.Store
	scheduler   scanTrigger
	identity    agent.Identity
	version     string
	hub         *Hub
	mux         *http.ServeMux
	warning     func() string
	addresses   func() []string
	sender      *webhooks.Sender
	internet    internetProvider
	certSHA256  string
	pairLimiter *ipRateLimiter
}

// NewServer construye el Server y registra todas las rutas.
func NewServer(deps Deps) *Server {
	if deps.Hub == nil {
		deps.Hub = NewHub()
	}
	if deps.Version == "" {
		deps.Version = "dev"
	}
	if deps.Addresses == nil {
		deps.Addresses = agent.LocalIPv4Addresses
	}
	if deps.Sender == nil {
		deps.Sender = webhooks.NewSender(deps.Version)
	}
	s := &Server{
		store:       deps.Store,
		scheduler:   deps.Scheduler,
		identity:    deps.Identity,
		version:     deps.Version,
		hub:         deps.Hub,
		warning:     deps.Warning,
		addresses:   deps.Addresses,
		sender:      deps.Sender,
		internet:    deps.Internet,
		certSHA256:  deps.CertSHA256,
		pairLimiter: newIPRateLimiter(pairRateLimitMax, pairRateLimitWindow),
	}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

// ServeHTTP hace de Server un http.Handler (para httptest y para
// http.Server.Handler en cmd/vigia-agent).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Hub expone el difusor de eventos SSE: cmd/vigia-agent conecta ahí lo que
// publica el scheduler, sin que este paquete dependa de él.
func (s *Server) Hub() *Hub { return s.hub }

func (s *Server) routes() {
	// Sin token (ver docs/agente-api.md): info (para saber a qué agente te
	// vas a vincular) y pair (todavía no hay token que mandar).
	s.mux.HandleFunc("GET /api/v1/info", s.handleInfo)
	s.mux.HandleFunc("POST /api/v1/pair", s.handlePair)

	// Interno: no es parte del contrato con la app. Solo loopback — lo usa
	// `vigia-agent pair` para pedirle un código al agente ya corriendo.
	s.mux.HandleFunc("POST /internal/pair-code", requireLoopback(s.handlePairCode))

	// Con token.
	s.mux.HandleFunc("GET /api/v1/networks", s.withAuth(s.handleNetworks))
	// Subtree a propósito: {key} puede traer "/" (la subred es un CIDR,
	// "192.168.0.0/24") percent-encoded, y el mux de la stdlib decodifica
	// %2F a "/" en r.URL.Path antes de hacer coincidir segmentos — un
	// patrón "{key}/devices" se rompería. handleNetworkDevices separa el
	// prefijo/sufijo sobre EscapedPath() (sin decodificar) y recién ahí
	// hace un único url.PathUnescape sobre lo de en medio.
	s.mux.HandleFunc("GET /api/v1/networks/", s.withAuth(s.handleNetworkDevices))

	s.mux.HandleFunc("GET /api/v1/devices/{id}", s.withAuth(s.handleGetDevice))
	s.mux.HandleFunc("GET /api/v1/devices/{id}/events", s.withAuth(s.handleDeviceEvents))
	s.mux.HandleFunc("PATCH /api/v1/devices/{id}", s.withAuth(s.handlePatchDevice))
	s.mux.HandleFunc("DELETE /api/v1/devices/{id}", s.withAuth(s.handleDeleteDevice))
	s.mux.HandleFunc("POST /api/v1/scan", s.withAuth(s.handleScan))
	s.mux.HandleFunc("GET /api/v1/stream", s.withAuth(s.handleStream))

	// Alertas (v1.1, ver docs/agente-api.md, sección Alertas).
	s.mux.HandleFunc("GET /api/v1/alerts", s.withAuth(s.handleAlerts))
	s.mux.HandleFunc("POST /api/v1/alerts/read", s.withAuth(s.handleAlertsRead))
	s.mux.HandleFunc("GET /api/v1/alert-settings", s.withAuth(s.handleAlertSettingsGet))
	s.mux.HandleFunc("PUT /api/v1/alert-settings", s.withAuth(s.handleAlertSettingsPut))
	s.mux.HandleFunc("GET /api/v1/webhooks", s.withAuth(s.handleWebhooksList))
	s.mux.HandleFunc("POST /api/v1/webhooks", s.withAuth(s.handleWebhooksCreate))
	s.mux.HandleFunc("DELETE /api/v1/webhooks/{id}", s.withAuth(s.handleWebhookDelete))
	s.mux.HandleFunc("POST /api/v1/webhooks/{id}/test", s.withAuth(s.handleWebhookTest))

	// Seguridad (v1.2, ver docs/agente-api.md).
	s.mux.HandleFunc("GET /api/v1/security", s.withAuth(s.handleSecurity))
	// Hallazgos reconocidos (v1.3). Subtree a propósito, igual que
	// "GET /api/v1/networks/" arriba: {findingId} puede traer "/" de
	// verdad (p. ej. "PORT_MAPPING|dev:12|7070/TCP") percent-encoded, y un
	// patrón "{findingId}" del mux se rompería apenas %2F se decodifique.
	s.mux.HandleFunc("PUT /api/v1/security/acknowledged/", s.withAuth(s.handleAcknowledgedPut))
	s.mux.HandleFunc("DELETE /api/v1/security/acknowledged/", s.withAuth(s.handleAcknowledgedDelete))

	// Internet (v1.4, ver docs/agente-api.md).
	s.mux.HandleFunc("GET /api/v1/internet", s.withAuth(s.handleInternet))
	s.mux.HandleFunc("GET /api/v1/internet/series", s.withAuth(s.handleInternetSeries))
	s.mux.HandleFunc("POST /api/v1/internet/speedtest", s.withAuth(s.handleInternetSpeedtest))
	s.mux.HandleFunc("GET /api/v1/internet/settings", s.withAuth(s.handleInternetSettingsGet))
	s.mux.HandleFunc("PUT /api/v1/internet/settings", s.withAuth(s.handleInternetSettingsPut))
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return requireToken(s.store, next)
}
