// Package core es el motor compartido de Vigía: descubre dispositivos en una red
// local, los reconoce (marca/modelo/tipo/SO) y escanea puertos. Corre igual en el
// agente 24/7 (macOS/Linux) y, vía gomobile, dentro de las apps móviles. No tiene estado:
// recibe una red y devuelve hallazgos.
package core

import (
	"context"
	"time"
)

// Device es un dispositivo encontrado en la red.
type Device struct {
	IP        string            `json:"ip"`                  // IPv4 principal
	IPv6      []string          `json:"ipv6,omitempty"`      // direcciones IPv6 vistas
	MAC       string            `json:"mac,omitempty"`       // normalizada "aa:bb:cc:dd:ee:ff"
	Vendor    string            `json:"vendor,omitempty"`    // fabricante por OUI
	Hostname  string            `json:"hostname,omitempty"`  // mejor nombre (mDNS > NetBIOS > PTR)
	Identity  Identity          `json:"identity"`            // resultado del reconocimiento
	Services  []Service         `json:"services,omitempty"`  // evidencia mDNS/SSDP/NetBIOS
	OpenPorts []int             `json:"openPorts,omitempty"` // puertos TCP abiertos
	Sources   []string          `json:"sources"`             // protocolos que lo vieron: arp, ping, mdns, ssdp, netbios, ndp
	Extra     map[string]string `json:"extra,omitempty"`     // datos sueltos (ej. ssdp:friendlyName)
	LastSeen  time.Time         `json:"lastSeen"`
}

// Identity es el reconocimiento: qué creemos que es el dispositivo y por qué.
// Confidence 0–100 e inspeccionable (Reason dice qué regla disparó): es la mejora
// deliberada sobre la nube opaca de Fing.
type Identity struct {
	Type       string `json:"type,omitempty"` // phone, computer, tv, router, printer, camera, speaker, console, iot, nas, unknown
	Brand      string `json:"brand,omitempty"`
	Model      string `json:"model,omitempty"`
	OS         string `json:"os,omitempty"`
	Confidence int    `json:"confidence"` // 0–100
	Reason     string `json:"reason,omitempty"`
}

// Service es una pista de un protocolo de descubrimiento.
type Service struct {
	Proto string `json:"proto"`          // mdns, ssdp, netbios
	Name  string `json:"name"`           // tipo de servicio / friendlyName / nombre NetBIOS
	Info  string `json:"info,omitempty"` // detalle (modelName, txt, etc.)
}

// MDNSInfo: lo que mDNS/Bonjour dijo de una IP.
type MDNSInfo struct {
	Hostname string   // p. ej. "Living-Room-TV.local"
	Services []string // tipos de servicio, p. ej. "_airplay._tcp", "_googlecast._tcp"
	Model    string   // del TXT: "model"/"md" (Chromecast/HomeKit), "am" (AirPlay), "rpMd" (companion-link)
	// OSVersion: TXT "osxvers" de _device-info (Darwin, crudo, p. ej. "24").
	// Viene de un servicio que mDNSResponder no resuelve con su propia cadena
	// PTR→SRV→A: cuela su TXT como adicional de CUALQUIER otra respuesta sobre
	// el mismo host (ver el comentario de discoverMDNS en discover.go). El
	// motor (recognize.go) la traduce a nombre comercial ("macOS 15 Sequoia").
	OSVersion string
	// Manufacturer: TXT "manufacturer="/"integrator=" (p. ej. una TV Samsung
	// anunciando _airplay._tcp trae "manufacturer=Samsung" sin SSDP de por medio).
	Manufacturer string
	// Instances: nombres humanos de instancia vistos para este host (p. ej.
	// "iMac de Estudio", `75" QLED`), sin duplicados ni identificadores internos
	// (ver humanInstanceName en discover_extra.go).
	Instances []string
	// Name: el mejor de Instances según el servicio que lo anunció (AirPlay,
	// _device-info… antes que el resto): el nombre visible del equipo.
	Name     string
	namePrio int
	Extra    map[string]string
}

// SSDPInfo: lo que SSDP/UPnP dijo de una IP (incluye el XML de descripción ya leído).
type SSDPInfo struct {
	Server       string // cabecera SERVER
	FriendlyName string // <friendlyName>
	Manufacturer string // <manufacturer>
	ModelName    string // <modelName>
	ModelNumber  string // <modelNumber>
	DeviceType   string // <deviceType>
	// UDN: <UDN> ("uuid:…"), identificador único del aparato UPnP: sirve para
	// reconocerlo aunque cambie de IP (historial de la app).
	UDN string
}

// NetBIOSInfo: respuesta a una consulta de nombre NetBIOS (UDP 137).
type NetBIOSInfo struct {
	Name string // nombre de equipo
	MAC  string // MAC incluida en la respuesta (útil cuando la tabla ARP no la da)
}

// TuyaInfo: lo que vimos en el broadcast UDP de un equipo Tuya (6666/6667).
type TuyaInfo struct {
	Version    string // "3.1" (puerto 6666, JSON en claro) o "3.3+" (puerto 6667, cifrado)
	GwID       string // "gwId" del JSON, solo disponible en 3.1 (6666)
	ProductKey string // "productKey" del JSON, solo disponible en 3.1 (6666)
}

// ScanResult es el resultado de un escaneo.
type ScanResult struct {
	Subnet    string        `json:"subnet"`
	Self      string        `json:"self"` // nuestra IP en esa red
	Gateway   string        `json:"gateway,omitempty"`
	Devices   []Device      `json:"devices"`
	StartedAt time.Time     `json:"startedAt"`
	Duration  time.Duration `json:"duration"`
}

// ScanOptions configura un escaneo.
type ScanOptions struct {
	// CIDR a escanear. Vacío = autodetectar la red de la interfaz por defecto.
	// En Android 11+ hay que pasarlo siempre: el sistema no deja a las apps
	// listar interfaces (netlink bloqueado), así que la app lo saca de
	// LinkProperties.
	CIDR string
	// Gateway (router) de la red. Vacío = autodetectar con el comando del SO,
	// que en Android no está disponible: la app lo pasa desde LinkProperties.
	Gateway string
	// Puertos TCP a probar por host. nil = DefaultPorts. Vacío ([]int{}) = no escanear puertos.
	Ports []int
	// Tiempos. Cero = valores por defecto.
	SweepTimeout    time.Duration // por host en el barrido de liveness (sondas en paralelo)
	DiscoverTimeout time.Duration // ventana de escucha mDNS/SSDP/Tuya y consulta NetBIOS
	PortTimeout     time.Duration // por puerto
	// Concurrencia del barrido/escaneo. Cero = DefaultConcurrency.
	Concurrency int
	// Routed, si es true, trata esta red como una red de delante del router
	// (el "anillo exterior" de la Fase 5: ver docs/plan.md y
	// docs/agente-api.md, sección Seguridad) en vez de la red local de
	// siempre: el multicast (mDNS/SSDP/Tuya) y la tabla ARP/NDP no cruzan un
	// router, así que Scan se los salta y se queda con barrido ICMP/TCP,
	// NetBIOS unicast, puertos y huella HTTP. false (el valor por defecto) no
	// fuerza nada: Scan detecta solo el mismo caso si CIDR no coincide con
	// ninguna interfaz local (ver isRoutedNetwork) — Routed=true sirve para
	// el caso en que sí coincide por casualidad, o para dejarlo explícito.
	Routed bool
	// SkipNeighborTable, si es true, no lee la tabla ARP/NDP del sistema
	// (neighborTable). Se activa en Android/iOS (donde el sistema la bloquea
	// para apps normales) y el descubrimiento se apoya en ICMP, NetBIOS,
	// mDNS/SSDP y Tuya. En escritorio también permite probar ese camino.
	SkipNeighborTable bool
	// BindInterface, si no está vacío (p. ej. "en0", "wlan0"), enlaza los
	// sockets del escaneo a esa interfaz (IP_BOUND_IF en Darwin/iOS,
	// SO_BINDTODEVICE en Linux/Android, con respaldo transparente si no
	// aplica) para escanear la LAN Wi-Fi incluso con un túnel VPN/Tailscale
	// activo.
	BindInterface string
	// Progreso opcional. El motor serializa las llamadas (con un mutex propio
	// si hace falta: varias etapas corren en paralelo), así que el callback
	// no necesita ser seguro para hilos. Puede ser nil. Etapas:
	//   "sweep"    barrido de la subred (done/total = IPs probadas)
	//   "names"    nombres y anuncios tras el barrido: NetBIOS, mDNS, SSDP,
	//              Tuya (total 0 = indeterminado) —
	//              corre en paralelo con "sweep" y el resto del
	//              descubrimiento (mDNS/SSDP/Tuya), no necesariamente después
	//   "identify" puertos y reconocimiento (done/total = dispositivos)
	OnProgress func(stage string, done, total int)
}

// DefaultPorts: puertos comunes que, abiertos o cerrados, confirman que el host
// está vivo y además alimentan el reconocimiento (iOS 62078, iTunes 3689, etc.).
var DefaultPorts = []int{
	22, 23, 53, 80, 139, 443, 445, 515, 548, 554, 631, 1900, 3689, 5000, 5353,
	7000, 8001, 8008, 8009, 8060, 8080, 8443, 9100, 32400, 49152, 62078,
}

// Los timeouts dejan pasar UNA retransmisión del SYN (el kernel la manda al
// segundo): medido en la red real, con 700 ms se perdían puertos abiertos de
// los aparatos por Wi-Fi (un SYN perdido o un equipo en ahorro de energía);
// con 1.5 s salieron todos en todas las rondas.
const (
	DefaultConcurrency     = 128
	DefaultSweepTimeout    = 1200 * time.Millisecond
	DefaultDiscoverTimeout = 4 * time.Second
	DefaultPortTimeout     = 1500 * time.Millisecond
)

// contexto corto para no bloquear si una etapa se cuelga.
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
