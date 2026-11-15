// Package mobile es la frontera entre el motor (core) y la app Android, vía
// gomobile (`gomobile bind`). gomobile solo exporta tipos simples (string, bool,
// int, error, interfaces y structs de este paquete): nada de slices ni mapas,
// así que los resultados viajan como JSON y la app los decodifica en Kotlin.
//
// Uso desde Kotlin (hilo de fondo, p. ej. Dispatchers.IO):
//
//	val scanner = Mobile.newScanner()
//	val json = scanner.scan("192.168.0.105/24", "192.168.0.4", true, listener)
//	scanner.cancel()  // desde otro hilo, para cortar
//
// En Android hay que pasar siempre la red (cidr) y el gateway: el sistema no
// deja a las apps listar interfaces ni consultar rutas. La app los saca de
// ConnectivityManager/LinkProperties, y además debe tener tomado un
// WifiManager.MulticastLock mientras escanea, o el Wi-Fi descarta mDNS/SSDP.
package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/baxner/vigia/core"
)

// Version del motor que lleva la app.
func Version() string { return "0.1.0" }

// ProgressListener lo implementa la app para ver el avance del escaneo.
// El motor serializa las llamadas. En Kotlin `done`/`total` llegan como Long.
type ProgressListener interface {
	OnProgress(stage string, done int, total int)
}

// Scanner es un escaneo cancelable. Uno por escaneo en curso.
type Scanner struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	seq    uint64
}

// NewScanner crea un Scanner.
func NewScanner() *Scanner { return &Scanner{} }

// ScanConfig configures a network scan, including optional interface binding
// (e.g. "en0" on iOS or "wlan0" on Android) so local LAN scanning bypasses VPN
// tunnels.
type ScanConfig struct {
	CIDR              string
	Gateway           string
	WithPorts         bool
	BindIface         string
	SkipNeighborTable bool
}

// NewScanConfig returns a ScanConfig with platform defaults pre-populated.
func NewScanConfig() *ScanConfig {
	return &ScanConfig{
		WithPorts:         true,
		SkipNeighborTable: runtime.GOOS == "android" || runtime.GOOS == "ios",
	}
}

// Scan escanea la red y devuelve el resultado (core.ScanResult) en JSON.
// Bloquea hasta terminar o hasta Cancel. cidr es obligatorio en Android
// (p. ej. "192.168.0.105/24"); gateway es opcional. withPorts escanea los
// puertos por defecto de cada dispositivo. listener puede ser nil.
func (s *Scanner) Scan(cidr, gateway string, withPorts bool, listener ProgressListener) (string, error) {
	return s.ScanOnInterface(cidr, gateway, withPorts, "", listener)
}

// ScanOnInterface is like Scan but allows binding scan sockets and HTTP probes
// to bindIface (e.g. "en0" on iOS) to keep local discovery on the Wi-Fi LAN.
func (s *Scanner) ScanOnInterface(cidr, gateway string, withPorts bool, bindIface string, listener ProgressListener) (string, error) {
	cfg := NewScanConfig()
	cfg.CIDR = cidr
	cfg.Gateway = gateway
	cfg.WithPorts = withPorts
	cfg.BindIface = bindIface
	return s.ScanWithConfig(cfg, listener)
}

// ScanWithConfig executes a scan using the supplied ScanConfig.
func (s *Scanner) ScanWithConfig(cfg *ScanConfig, listener ProgressListener) (string, error) {
	if cfg == nil {
		cfg = NewScanConfig()
	}
	ctx, cancel, seq := s.begin()
	defer s.end(seq, cancel)

	opts := core.ScanOptions{
		CIDR:              cfg.CIDR,
		Gateway:           cfg.Gateway,
		BindInterface:     cfg.BindIface,
		SkipNeighborTable: cfg.SkipNeighborTable || runtime.GOOS == "android" || runtime.GOOS == "ios",
	}
	if !cfg.WithPorts {
		opts.Ports = []int{}
	}
	if listener != nil {
		opts.OnProgress = func(stage string, done, total int) {
			listener.OnProgress(stage, done, total)
		}
	}
	res, err := core.Scan(ctx, opts)
	if err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", errors.New("escaneo cancelado")
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ScanPorts escanea los puertos de un solo host y devuelve los abiertos como
// JSON (lista de enteros). spec: "" = puertos por defecto; si no, lista y/o
// rangos separados por comas, p. ej. "22,80,443,8000-8100". Máximo 65535.
func (s *Scanner) ScanPorts(ip string, spec string) (string, error) {
	return s.ScanPortsOnInterface(ip, spec, "")
}

// ScanPortsOnInterface is like ScanPorts but binds TCP dials to bindIface
// when non-empty.
func (s *Scanner) ScanPortsOnInterface(ip string, spec string, bindIface string) (string, error) {
	ports, err := parsePorts(spec)
	if err != nil {
		return "", err
	}
	ctx, cancel, seq := s.begin()
	defer s.end(seq, cancel)
	open := core.ScanHostPortsOnInterface(ctx, ip, ports, 0, bindIface)
	if errors.Is(ctx.Err(), context.Canceled) {
		return "", errors.New("escaneo cancelado") // resultados a medias: mejor ninguno
	}
	if open == nil {
		open = []int{}
	}
	b, err := json.Marshal(open)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Cancel corta el escaneo en curso (si hay uno). Seguro desde cualquier hilo.
func (s *Scanner) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

// VendorForMAC devuelve el fabricante de una MAC (OUI), o "" si no se conoce o
// es aleatoria. La app lo usa también con los BSSID del escáner Wi-Fi.
func VendorForMAC(mac string) string {
	return core.VendorForMAC(mac)
}

// BrandForMAC: la marca limpia del fabricante de una MAC/BSSID, o "".
func BrandForMAC(mac string) string {
	return core.BrandForMAC(mac)
}

// PortCatalog devuelve el catálogo de puertos del motor (core/data/ports.tsv,
// ver core/ports.go) como JSON: [{"port","name","risk","why","action"}, …].
// No puede fallar (son datos fijos, embebidos en el binario): si
// json.Marshal alguna vez diera error sería un bug de este código, no algo
// que el llamador deba manejar — por eso, a diferencia de RouterInfo/
// SecurityReport, no devuelve error (mismo contrato que
// docs/agente-api.md: "Mobile.portCatalog(): String").
func PortCatalog() string {
	b, err := json.Marshal(core.PortCatalog())
	if err != nil {
		return "[]" // nunca debería pasar; ver comentario de arriba
	}
	return string(b)
}

// RouterInfo averigua lo que se pueda del router de gateway por UPnP y
// NAT-PMP (ver core.QueryRouter) y lo devuelve como JSON de
// core.RouterInfo. Si el router no contesta ninguno de los dos, devuelve
// la cadena "null" (sin error: no contestar es el caso normal de la
// mayoría de los routers, ver docs/agente-api.md). core.QueryRouter ya se
// acota sola a unos segundos (no cuelga el hilo de la app), así que no
// hace falta un timeout propio aquí ni un Scanner cancelable: es una sola
// llamada de ida y vuelta.
func RouterInfo(gateway string) (string, error) {
	return RouterInfoOnInterface(gateway, "")
}

// RouterInfoOnInterface is like RouterInfo but binds UPnP/NAT-PMP sockets to
// bindIface when non-empty.
func RouterInfoOnInterface(gateway, bindIface string) (string, error) {
	info, err := core.QueryRouterOnInterface(context.Background(), gateway, bindIface)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "null", nil
	}
	b, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SecurityReport decodifica input (JSON de core.SecurityInput: devices,
// router, outerRing, wifi, recentAlerts — ver docs/agente-api.md, sección
// Seguridad) y devuelve el informe (core.SecurityReport) como JSON,
// calculado con la hora actual del teléfono.
func SecurityReport(input string) (string, error) {
	var in core.SecurityInput
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return "", err
	}
	report := core.BuildSecurityReport(in, time.Now())
	b, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Scanner) begin() (context.Context, context.CancelFunc, uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel() // un escaneo nuevo corta el anterior del mismo Scanner
	}
	s.seq++
	seq := s.seq
	s.cancel = cancel
	s.mu.Unlock()
	return ctx, cancel, seq
}

func (s *Scanner) end(seq uint64, cancel context.CancelFunc) {
	if cancel != nil {
		cancel()
	}
	s.mu.Lock()
	if s.seq == seq {
		s.cancel = nil
	}
	s.mu.Unlock()
}

// parsePorts interpreta "22,80,8000-8100". Vacío = core.DefaultPorts.
func parsePorts(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return append([]int(nil), core.DefaultPorts...), nil
	}
	seen := map[int]bool{}
	var out []int
	add := func(p int) error {
		if p < 1 || p > 65535 {
			return errors.New("puerto fuera de rango: " + strconv.Itoa(p))
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
		return nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if a, b, ok := strings.Cut(part, "-"); ok {
			lo, err1 := strconv.Atoi(strings.TrimSpace(a))
			hi, err2 := strconv.Atoi(strings.TrimSpace(b))
			if err1 != nil || err2 != nil || lo > hi {
				return nil, errors.New("rango inválido: " + part)
			}
			for p := lo; p <= hi; p++ {
				if err := add(p); err != nil {
					return nil, err
				}
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil {
			return nil, errors.New("puerto inválido: " + part)
		}
		if err := add(p); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no hay puertos que escanear")
	}
	return out, nil
}
