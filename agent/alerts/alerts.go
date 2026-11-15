// Package alerts implementa las reglas de "Alertas (v1.1)" del contrato
// (docs/agente-api.md): puro, sin IO ni reloj propio (quien llama pasa
// `at`), igual que agent/history — para poder probarlo con datos en
// memoria y sin una base real. Decide QUÉ alertas corresponden y con qué
// título/cuerpo en español de México; agent/store se encarga de
// persistirlas (con la idempotencia que da DedupKey) y cmd/vigia-agent de
// publicarlas por SSE y mandarlas a los webhooks.
package alerts

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/internet"
	"github.com/baxner/vigia/core"
)

// Type enumera los tipos de alerta del contrato.
type Type string

const (
	NewDevice        Type = "NEW_DEVICE"
	ImportantOffline Type = "IMPORTANT_OFFLINE"
	ImportantOnline  Type = "IMPORTANT_ONLINE"
	AgentBlind       Type = "AGENT_BLIND"
	AgentRecovered   Type = "AGENT_RECOVERED"
	// Fase 5 (ver docs/agente-api.md, "Seguridad v1.2" > Alertas nuevas):
	// ninguna de las cuatro nace de un EventInput de EvaluateScan.
	// ROUTER_CHANGED/ARP_CONFLICT salen de un escaneo del anillo interior
	// (EvaluateRouterChanged/EvaluateARPConflict); PORT_MAPPING_ADDED, de
	// una consulta al router (EvaluateNewPortMappings); RISKY_SERVICE, de
	// los puertos abiertos de un equipo ya conocido (EvaluateNewRiskyServices).
	RouterChanged    Type = "ROUTER_CHANGED"
	ArpConflict      Type = "ARP_CONFLICT"
	PortMappingAdded Type = "PORT_MAPPING_ADDED"
	RiskyService     Type = "RISKY_SERVICE"
	// Fase 7 (ver docs/agente-api.md, "Internet (v1.4)" > Caídas): las dos
	// nacen de agent/internet.OutageDetector (vía cmd/vigia-agent/internet.go),
	// no de un escaneo de core.Scan.
	InternetDown     Type = "INTERNET_DOWN"
	InternetRestored Type = "INTERNET_RESTORED"
	// Test no es un tipo de alerta real (este paquete nunca lo produce, y
	// no respeta Settings ni el filtro `types` de un webhook en
	// particular): lo usan POST /api/v1/webhooks/{id}/test y
	// `vigia-agent webhook test` para una entrega de prueba (ver
	// docs/agente-api.md, sección Webhooks, y TestAlert más abajo). Vive
	// aquí solo para reutilizar el mismo tipo Type en los dos lugares que
	// lo mandan (agent/api y cmd/vigia-agent).
	Test Type = "TEST"
)

// Severity enumera las severidades del contrato.
type Severity string

const (
	High   Severity = "high"
	Medium Severity = "medium"
	Low    Severity = "low"
)

// Settings son los interruptores de GET/PUT /api/v1/alert-settings: qué
// tipos de alerta están encendidos. Los tags JSON son el contrato tal
// cual (mismas claves que los valores de Type), igual que history.KnownDevice
// y los demás tipos de dominio que la API sirve sin un DTO aparte.
type Settings struct {
	NewDevice        bool `json:"NEW_DEVICE"`
	ImportantOffline bool `json:"IMPORTANT_OFFLINE"`
	ImportantOnline  bool `json:"IMPORTANT_ONLINE"`
	AgentBlind       bool `json:"AGENT_BLIND"`
	AgentRecovered   bool `json:"AGENT_RECOVERED"`
	// Fase 5 (ver docs/agente-api.md, "Seguridad v1.2").
	RouterChanged    bool `json:"ROUTER_CHANGED"`
	ArpConflict      bool `json:"ARP_CONFLICT"`
	PortMappingAdded bool `json:"PORT_MAPPING_ADDED"`
	RiskyService     bool `json:"RISKY_SERVICE"`
	// Fase 7 (ver docs/agente-api.md, "Internet (v1.4)").
	InternetDown     bool `json:"INTERNET_DOWN"`
	InternetRestored bool `json:"INTERNET_RESTORED"`
}

// DefaultSettings: todo encendido (ver migration2/migration3/migration5 en agent/store).
func DefaultSettings() Settings {
	return Settings{
		NewDevice: true, ImportantOffline: true, ImportantOnline: true, AgentBlind: true, AgentRecovered: true,
		RouterChanged: true, ArpConflict: true, PortMappingAdded: true, RiskyService: true,
		InternetDown: true, InternetRestored: true,
	}
}

// Enabled dice si el tipo t está encendido en s. false para cualquier
// tipo que no sea una de las nueve reglas reales (p. ej. Test).
func (s Settings) Enabled(t Type) bool {
	switch t {
	case NewDevice:
		return s.NewDevice
	case ImportantOffline:
		return s.ImportantOffline
	case ImportantOnline:
		return s.ImportantOnline
	case AgentBlind:
		return s.AgentBlind
	case AgentRecovered:
		return s.AgentRecovered
	case RouterChanged:
		return s.RouterChanged
	case ArpConflict:
		return s.ArpConflict
	case PortMappingAdded:
		return s.PortMappingAdded
	case RiskyService:
		return s.RiskyService
	case InternetDown:
		return s.InternetDown
	case InternetRestored:
		return s.InternetRestored
	default:
		return false
	}
}

// Alert es una alerta evaluada por las reglas, todavía sin persistir (sin
// id ni read: eso lo decide agent/store al guardarla — ver store.Alert,
// que además trae los tags JSON del contrato).
type Alert struct {
	Type     Type
	Severity Severity
	At       time.Time
	Title    string
	Body     string
	// DeviceID: nil en AGENT_BLIND/AGENT_RECOVERED (ver contrato: "null en
	// AGENT_*").
	DeviceID *int64
	Network  string
	// DedupKey identifica el evento de origen: agent/store la guarda con
	// una restricción UNIQUE, así que reevaluar el mismo evento (reintento,
	// reinicio a mitad de un escaneo) nunca duplica la alerta — "Una
	// alerta por evento" del contrato se cumple en la capa de persistencia,
	// no confiando en que quien llama nunca repita una evaluación.
	DedupKey string
}

// newDeviceGrace: margen tras el primer escaneo de la red dentro del cual
// un alta todavía es "línea base", no un equipo nuevo. Mismo valor y
// mismo criterio que NEW_AFTER_BASELINE en
// android/.../ui/devices/DeviceTypeCatalog.kt (insignia "Nuevo" de la
// app) — ver también BaselineOf más abajo.
const newDeviceGrace = 15 * time.Minute

// EventInput es un evento de un escaneo listo para evaluar contra las
// reglas: el DeviceEvent tal cual y el KnownDevice ya actualizado al que
// corresponde (de donde salen custom.important y el nombre visible para
// el cuerpo de la alerta). Ver store.RecordedEvent, que trae exactamente
// esta pareja.
type EventInput struct {
	Event  history.DeviceEvent
	Device history.KnownDevice
}

// BaselineOf es el firstSeen más antiguo entre devices: la línea base de
// una red (ver EvaluateScan). Zero-value (time.Time{}) si devices está
// vacío. Puerto directo de baselineOf en DeviceTypeCatalog.kt.
func BaselineOf(devices []history.KnownDevice) time.Time {
	var baseline time.Time
	for _, d := range devices {
		if baseline.IsZero() || d.FirstSeen.Before(baseline) {
			baseline = d.FirstSeen
		}
	}
	return baseline
}

// Ring distingue, solo para el texto de NEW_DEVICE, si el escaneo es del
// anillo interior o del exterior (ver docs/agente-api.md, "Seguridad v1.2"
// > Anillos): "un equipo nuevo" dentro de casa no es lo mismo que uno nuevo
// en la red del módem, de delante del router.
type Ring int

const (
	RingInner Ring = iota
	RingOuter
)

// EvaluateScan aplica la regla NEW_DEVICE (ver docs/agente-api.md,
// "Alertas") a los eventos de UN escaneo de network, en el mismo orden
// que events. IMPORTANT_OFFLINE/IMPORTANT_ONLINE YA NO salen de aquí
// desde la Fase 5.1 (ver docs/plan.md): WENT_OFFLINE/CAME_ONLINE siguen
// registrándose igual en el historial (agent/history), pero las alertas de
// equipos importantes se evalúan aparte, en CADA escaneo y con su propio
// umbral de 15 min (ver EvaluateImportantOffline/EvaluateImportantOnline
// más abajo) — el umbral de gracia de WENT_OFFLINE (10 min o 2 escaneos,
// ver agent/history.graceWindow) es más corto y, atado a un Wi-Fi que se
// duerme de madrugada, avisaba de más. baseline es el
// firstSeen más antiguo entre los equipos conocidos de esa red DESPUÉS de
// este escaneo (línea base: ver BaselineOf, mismo criterio que la
// insignia "Nuevo" de la app) — su zero-value (baseline.IsZero()) se trata
// como "todavía no hay línea base" y por seguridad NUNCA dispara
// NEW_DEVICE (evita una tormenta de alertas si quien llama no pudo
// calcularla); en la práctica no debería pasar: el propio evento
// FIRST_SEEN ya insertó al menos un equipo en la red antes de que quien
// llama calcule baseline. ring solo cambia el cuerpo de NEW_DEVICE (ver
// Ring): esta misma función sirve para los dos anillos (Fase 5,
// docs/plan.md: "NEW_DEVICE también aplica al anillo exterior,
// con su propia línea base").
func EvaluateScan(events []EventInput, network string, baseline time.Time, settings Settings, ring Ring) []Alert {
	var out []Alert
	for _, in := range events {
		switch in.Event.Type {
		case history.FirstSeen:
			if !settings.NewDevice {
				continue
			}
			if baseline.IsZero() || in.Event.At.Sub(baseline) <= newDeviceGrace {
				continue // línea base, no es un equipo "nuevo"
			}
			body := label(in.Device) + " se conectó por primera vez."
			if ring == RingOuter {
				body = label(in.Device) + " se conectó por primera vez a la red de delante de tu router (la del módem)."
			}
			out = append(out, deviceAlert(NewDevice, High, "Equipo nuevo en tu red", body, in, network))
		}
	}
	return out
}

func deviceAlert(t Type, sev Severity, title, body string, in EventInput, network string) Alert {
	id := in.Event.DeviceID
	return Alert{
		Type: t, Severity: sev, At: in.Event.At, Title: title, Body: body,
		DeviceID: &id, Network: network,
		DedupKey: dedupKey(t, strconv.FormatInt(id, 10), in.Event.At),
	}
}

// ---------------------------------------------------------------------------
// IMPORTANT_OFFLINE / IMPORTANT_ONLINE (Fase 5.1, ver docs/plan.md:
// "Menos ruido en IMPORTANT_OFFLINE/IMPORTANT_ONLINE"). Datos reales de
// prueba: un teléfono marcado como importante dio 16 avisos en 3
// días porque su Wi-Fi se duerme de madrugada y vuelve a los pocos
// minutos — el umbral de gracia de WENT_OFFLINE (10 min o 2 escaneos) era
// demasiado corto para eso. A diferencia de v1.1 (atado al evento
// WENT_OFFLINE/CAME_ONLINE del historial), estas dos se evalúan mirando el
// estado ACTUAL de los equipos importantes en CADA escaneo del anillo
// interior.
// ---------------------------------------------------------------------------

// importantOfflineThreshold: cuánto debe llevar un equipo importante sin
// verse para avisar IMPORTANT_OFFLINE — más alto que la gracia de
// WENT_OFFLINE del historial (agent/history.graceWindow, 10 min) a
// propósito (ver el comentario de arriba).
const importantOfflineThreshold = 15 * time.Minute

// EvaluateImportantOffline avisa de los equipos importantes que llevan
// >= 15 min sin verse (now - LastSeen), UNA vez por "episodio": mientras un
// equipo siga sin conexión, su LastSeen no cambia (ver history.Missing), así
// que volver a llamar esta función en cada escaneo siguiente arma la MISMA
// Alert (misma DedupKey) — es la restricción UNIQUE de agent/store sobre
// DedupKey la que de verdad evita que se repita el aviso, no esta función
// (mismo patrón que ARP_CONFLICT/PORT_MAPPING_ADDED). Quien llama debe
// marcar el episodio como pendiente de su IMPORTANT_ONLINE (ver
// agent/store.SetImportantOfflinePending) por cada Alert que esta función
// devuelva, se haya terminado creando o no en la base: la condición ya se
// cumplió, y es lo único que EvaluateImportantOnline necesita para decidir
// si el regreso de ese equipo corresponde a un episodio que sí se avisó.
func EvaluateImportantOffline(devices []history.KnownDevice, now time.Time, network string, settings Settings) []Alert {
	if !settings.ImportantOffline {
		return nil
	}
	var out []Alert
	for _, d := range devices {
		if !d.Custom.Important || d.Online {
			continue
		}
		if now.Sub(d.LastSeen) < importantOfflineThreshold {
			continue
		}
		out = append(out, importantOfflineAlert(d, network))
	}
	return out
}

func importantOfflineAlert(d history.KnownDevice, network string) Alert {
	id := d.ID
	return Alert{
		Type: ImportantOffline, Severity: Medium, At: d.LastSeen,
		Title:    "Equipo importante desconectado",
		Body:     label(d) + " lleva más de 15 minutos sin conexión.",
		DeviceID: &id, Network: network,
		// Por (equipo, LastSeen): el episodio, no el escaneo que lo nota.
		DedupKey: dedupKey(ImportantOffline, strconv.FormatInt(d.ID, 10), d.LastSeen),
	}
}

// EvaluateImportantOnline avisa IMPORTANT_ONLINE solo de los equipos
// importantes que están en línea AHORA y tienen un episodio
// IMPORTANT_OFFLINE pendiente (pending, ver
// agent/store.ImportantOfflinePending): sin eso, un regreso que nunca se
// avisó como caída (el Wi-Fi se durmió menos de 15 min, o el ajuste estaba
// apagado cuando se fue) tampoco avisa su regreso. resolved lista TODOS los
// equipos pendientes que ya están en línea, aunque settings.ImportantOnline
// esté apagado (el ajuste silencia la ALERTA, no el estado — mismo criterio
// que RISKY_SERVICE): quien llama debe limpiar el pendiente (ver
// agent/store.ClearImportantOfflinePending) de cada uno de resolved, para
// que el PRÓXIMO episodio pueda avisar de nuevo.
func EvaluateImportantOnline(devices []history.KnownDevice, pending map[int64]bool, now time.Time, network string, settings Settings) (out []Alert, resolved []int64) {
	for _, d := range devices {
		if !d.Custom.Important || !d.Online || !pending[d.ID] {
			continue
		}
		resolved = append(resolved, d.ID)
		if !settings.ImportantOnline {
			continue
		}
		out = append(out, importantOnlineAlert(d, network, now))
	}
	return out, resolved
}

func importantOnlineAlert(d history.KnownDevice, network string, at time.Time) Alert {
	id := d.ID
	return Alert{
		Type: ImportantOnline, Severity: Low, At: at,
		Title:    "Equipo importante en línea",
		Body:     label(d) + " volvió a estar en línea.",
		DeviceID: &id, Network: network,
		DedupKey: dedupKey(ImportantOnline, strconv.FormatInt(d.ID, 10), at),
	}
}

// EvaluateHealth decide la transición ciego↔normal del agente (AGENT_BLIND
// / AGENT_RECOVERED). wasBlind es el estado persistido ANTES de este
// escaneo (ver store.Store.AgentHealthState — sobrevive reinicios, a
// propósito: "una sola alerta hasta que se recupere" no puede depender de
// memoria de proceso); isBlind, el de este escaneo. warning es el texto
// ya redactado que explica por qué el agente está ciego (ver
// noLocalNetworkWarning en cmd/vigia-agent/run.go, el mismo que ve
// /api/v1/info): se reutiliza tal cual como body de AGENT_BLIND en vez de
// duplicar la redacción.
//
// transitioned indica si quien llama debe persistir isBlind como el nuevo
// estado — SIEMPRE que wasBlind != isBlind, sin importar si el tipo
// correspondiente está apagado en settings: perder la transición dejaría
// una alerta pendiente de más o de menos el día que el ajuste se
// reactive. alert es nil si no hubo transición, o si el tipo
// correspondiente está apagado.
func EvaluateHealth(wasBlind, isBlind bool, warning, network string, at time.Time, settings Settings) (transitioned bool, alert *Alert) {
	if wasBlind == isBlind {
		return false, nil
	}
	if isBlind {
		if !settings.AgentBlind {
			return true, nil
		}
		body := warning
		if body == "" {
			body = "El agente dejó de recibir respuesta de los equipos de tu red."
		}
		a := Alert{Type: AgentBlind, Severity: Medium, At: at, Title: "Vigía dejó de ver tu red", Body: body,
			Network: network, DedupKey: dedupKey(AgentBlind, network, at)}
		return true, &a
	}
	if !settings.AgentRecovered {
		return true, nil
	}
	a := Alert{Type: AgentRecovered, Severity: Low, At: at, Title: "Vigía vuelve a ver tu red",
		Body: "El agente volvió a recibir respuesta de los equipos de tu red.", Network: network,
		DedupKey: dedupKey(AgentRecovered, network, at)}
	return true, &a
}

// TestAlert es la alerta sintética de una entrega de prueba (ver
// POST /api/v1/webhooks/{id}/test y `vigia-agent webhook test`): nunca se
// persiste, no respeta Settings ni el filtro `types` de ningún webhook en
// particular — probar un webhook debe mandarle algo sin importar a qué
// está suscrito.
func TestAlert(at time.Time) Alert {
	return Alert{Type: Test, Severity: Low, At: at, Title: "Alerta de prueba",
		Body: "Esto es una alerta de prueba de Vigía."}
}

// dedupKey identifica el evento de origen de una alerta: tipo + (equipo o
// red, según corresponda) + instante. Ver el comentario de Alert.DedupKey.
func dedupKey(t Type, scope string, at time.Time) string {
	return fmt.Sprintf("%s|%s|%s", t, scope, at.UTC().Format(time.RFC3339Nano))
}

// dedupKeyDay: como dedupKey, pero a resolución de día (UTC) en vez de
// instante — para alertas que deben "dedup por día" (ver ARP_CONFLICT en
// docs/plan.md, Fase 5): mientras la condición siga presente,
// como máximo una alerta cada 24 horas, no una por escaneo.
func dedupKeyDay(t Type, scope string, at time.Time) string {
	return fmt.Sprintf("%s|%s|%s", t, scope, at.UTC().Format("2006-01-02"))
}

// firstNonEmpty devuelve el primer valor no vacío de vals, o "" si todos lo
// están. Puerto mínimo del helper homónimo de core/strutil.go (no
// exportado ahí): agent/alerts no puede importarlo y no vale la pena
// exportarlo solo por esto.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// ROUTER_CHANGED (ver docs/agente-api.md, "Seguridad v1.2" > Alertas nuevas)
// ---------------------------------------------------------------------------

// EvaluateRouterChanged decide si la MAC de la puerta de enlace del anillo
// interior cambió respecto de la guardada. previousMAC/currentMAC son las
// MAC ya normalizadas (ver core.Device.MAC) de ANTES y DESPUÉS de este
// escaneo — quien llama las captura leyendo y luego actualizando
// store.Network.GatewayMAC (ver cmd/vigia-agent/run.go: hay que leer la
// anterior ANTES de que RecordScan la sobrescriba). "" en cualquiera de
// las dos significa "no se sabe" (el ARP no resolvió esta vez, o es la
// primera vez que se ve esta red) y nunca genera alerta: una MAC
// desconocida no es una MAC distinta, y arriesgarse a avisar por un ARP
// que no contestó sería una falsa alarma (mismo principio que la baseline
// de NEW_DEVICE). Una alerta por cambio (dedup por instante, ver
// dedupKey): si vuelve a cambiar en el siguiente escaneo, avisa otra vez.
func EvaluateRouterChanged(previousMAC, currentMAC, network string, at time.Time, settings Settings) *Alert {
	if !settings.RouterChanged {
		return nil
	}
	if previousMAC == "" || currentMAC == "" || previousMAC == currentMAC {
		return nil
	}
	a := Alert{
		Type: RouterChanged, Severity: High, At: at,
		Title: "La puerta de enlace de tu red cambió",
		Body: "La MAC de tu router cambió. Puede ser que lo hayas reemplazado o reiniciado con otra interfaz, " +
			"pero también puede ser alguien haciéndose pasar por él.",
		Network:  network,
		DedupKey: dedupKey(RouterChanged, network, at),
	}
	return &a
}

// ---------------------------------------------------------------------------
// ARP_CONFLICT (ver docs/agente-api.md, "Seguridad v1.2" > Alertas nuevas)
// ---------------------------------------------------------------------------

// EvaluateARPConflict decide si, en ESTE escaneo del anillo interior, la IP
// del router respondió con la MAC de OTRO equipo ya conocido (alguien se
// hace pasar por el router). scanned es res.Devices tal cual de este
// escaneo (para encontrar quién respondió en gatewayIP); known son los
// equipos YA conocidos de esta red (antes o después de este escaneo da
// igual para esto: solo importan MAC y LastIP). Un equipo conocido cuya
// LastIP sea justo gatewayIP no cuenta como "otro" — es, lo más probable,
// la fila del propio router (si llegó a aparecer como "equipo" por algún
// otro protocolo). Dedup por día (ver dedupKeyDay): mientras el conflicto
// siga, como máximo una alerta cada 24 horas.
func EvaluateARPConflict(gatewayIP string, scanned []core.Device, known []history.KnownDevice, network string, at time.Time, settings Settings) *Alert {
	if !settings.ArpConflict || gatewayIP == "" {
		return nil
	}
	var gatewayMAC string
	for _, d := range scanned {
		if d.IP == gatewayIP {
			gatewayMAC = d.MAC
			break
		}
	}
	if gatewayMAC == "" {
		return nil
	}
	for _, k := range known {
		if k.LastIP == gatewayIP {
			continue
		}
		if k.MAC != nil && *k.MAC == gatewayMAC {
			a := Alert{
				Type: ArpConflict, Severity: High, At: at,
				Title: "Un equipo respondió como si fuera el router",
				Body: "Un equipo de tu red contestó con la misma IP que tu router, pero con otra MAC. " +
					"Puede ser un problema de red, pero también alguien tratando de interceptar tu tráfico.",
				Network:  network,
				DedupKey: dedupKeyDay(ArpConflict, network, at),
			}
			return &a
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// PORT_MAPPING_ADDED (ver docs/agente-api.md, "Seguridad v1.2" > Alertas nuevas)
// ---------------------------------------------------------------------------

// PortMappingKey identifica una redirección sin depender de los campos que
// pueden cambiar sin que sea "otra" redirección (descripción, deviceId):
// protocolo + puerto exterior + IP interior, igual que la clave de la tabla
// nueva de agent/store (ver docs/plan.md, Fase 5). La usa
// agent/store.ReconcilePortMappings para el historial de redirecciones
// ACTIVAS; no tiene nada que ver con PortMappingNotifyKey, que decide a
// quién avisar (ver más abajo).
type PortMappingKey struct {
	Protocol     string
	ExternalPort int
	InternalIP   string
}

// PortMappingKeyOf extrae la PortMappingKey de un core.PortMapping.
// Protocol se normaliza a mayúsculas y "" cae a "TCP" (ver core.PortMapping:
// el campo es "TCP"|"UDP", pero más vale no arriesgarse a una clave
// distinta por un protocolo vacío de un router raro).
func PortMappingKeyOf(p core.PortMapping) PortMappingKey {
	return PortMappingKey{
		Protocol:     strings.ToUpper(firstNonEmpty(p.Protocol, "TCP")),
		ExternalPort: p.ExternalPort,
		InternalIP:   p.InternalIP,
	}
}

// versionOrLoneNumberRe: un token (separado por espacios) que es SOLO un
// número de versión ("3.2.5", "v2") o un número suelto ("844780211") — ni
// uno identifica el programa, así que NormalizeDescription los quita (ver
// docs/plan.md, Fase 5.1: "sin números de versión ni números sueltos").
var versionOrLoneNumberRe = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+)*$`)

// NormalizeDescription reduce la descripción que un equipo mandó al pedir
// una redirección UPnP a su "programa": minúsculas, sin números de versión
// ni sueltos (ver docs/plan.md, Fase 5.1 y versionOrLoneNumberRe).
// "HyperTorrent 3.2.5" -> "hypertorrent"; "NAT-PMP 844780211" -> "nat-pmp";
// "tailscale-portmap" y "AnyDesk" se quedan igual (en minúsculas). ""
// (sin descripción, o solo números tras normalizar) significa "no hay
// programa que identifique" — PortMappingNotifyKeyOf cae entonces al
// esquema de antes (protocolo + IP + puerto interior).
func NormalizeDescription(desc string) string {
	desc = strings.ToLower(strings.TrimSpace(desc))
	if desc == "" {
		return ""
	}
	fields := strings.Fields(desc)
	kept := make([]string, 0, len(fields))
	for _, f := range fields {
		if versionOrLoneNumberRe.MatchString(f) {
			continue
		}
		kept = append(kept, f)
	}
	return strings.Join(kept, " ")
}

// PortMappingNotifyKey identifica a QUIÉN avisarle una redirección nueva
// (ver docs/plan.md, Fase 5.1: "Menos ruido en PORT_MAPPING_ADDED"): equipo
// (IP interior) + programa (ver NormalizeDescription) cuando el router dio
// una descripción — así TCP y UDP de la misma petición (HyperTorrent pide
// las dos a la vez) y sesiones sucesivas con un puerto interior distinto
// (HyperTorrent cambia de puerto en cada sesión) caen en la MISMA clave y
// avisan una sola vez. Sin descripción (o si no quedó nada tras
// normalizarla), cae al esquema de antes: protocolo + IP + puerto interior
// (Protocol/InternalPort), para no perder la distinción cuando no hay
// programa que la reemplace.
type PortMappingNotifyKey struct {
	InternalIP   string
	Program      string // "" si no hay descripción utilizable: ver Protocol/InternalPort.
	Protocol     string
	InternalPort int
}

// PortMappingNotifyKeyOf extrae la PortMappingNotifyKey de un
// core.PortMapping (ver el comentario del tipo).
func PortMappingNotifyKeyOf(p core.PortMapping) PortMappingNotifyKey {
	if prog := NormalizeDescription(p.Description); prog != "" {
		return PortMappingNotifyKey{InternalIP: p.InternalIP, Program: prog}
	}
	return PortMappingNotifyKey{
		InternalIP:   p.InternalIP,
		Protocol:     strings.ToUpper(firstNonEmpty(p.Protocol, "TCP")),
		InternalPort: p.InternalPort,
	}
}

// EvaluateNewPortMappings avisa (PORT_MAPPING_ADDED) la primera vez que un
// equipo + programa (ver PortMappingNotifyKeyOf) pide una redirección:
// seen son las claves que ya se vieron alguna vez en esta red (activas o
// no — ver agent/store.PortMappingNotifyKeysSeen, que las deriva de TODO lo
// que ya está en port_mappings, no solo de lo visto desde que existe esta
// regla). Agrupa las redirecciones ACTUALES por la misma clave antes de
// avisar: así una sesión que pide TCP y UDP a la vez hacia el mismo
// programa manda UN solo aviso, y cambiar de puerto interior entre
// sesiones (mismo equipo, mismo programa) no vuelve a avisar. Sin línea
// base (primera corrida), nunca avisa.
func EvaluateNewPortMappings(seen map[PortMappingNotifyKey]bool, current []core.PortMapping, hasBaseline bool, network string, at time.Time, settings Settings) []Alert {
	if !hasBaseline || !settings.PortMappingAdded {
		return nil
	}
	groups := make(map[PortMappingNotifyKey][]core.PortMapping)
	var order []PortMappingNotifyKey
	for _, p := range current {
		k := PortMappingNotifyKeyOf(p)
		if seen[k] {
			continue
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p)
	}
	var out []Alert
	for _, k := range order {
		out = append(out, portMappingAddedAlert(k, groups[k], network, at))
	}
	return out
}

// portMappingAddedAlert redacta el aviso de UN grupo (misma
// PortMappingNotifyKey): mappings trae una o más core.PortMapping (p. ej.
// TCP y UDP de la misma petición) y nunca viene vacío.
func portMappingAddedAlert(k PortMappingNotifyKey, mappings []core.PortMapping, network string, at time.Time) Alert {
	sorted := append([]core.PortMapping(nil), mappings...)
	sort.Slice(sorted, func(i, j int) bool {
		pi, pj := strings.ToUpper(firstNonEmpty(sorted[i].Protocol, "TCP")), strings.ToUpper(firstNonEmpty(sorted[j].Protocol, "TCP"))
		if pi != pj {
			return pi < pj
		}
		return sorted[i].ExternalPort < sorted[j].ExternalPort
	})

	sev := Medium
	var deviceID *int64
	var desc string
	parts := make([]string, 0, len(sorted))
	for _, m := range sorted {
		if entry, ok := core.LookupPort(m.InternalPort); ok && entry.Risk == "high" {
			sev = High
		}
		if deviceID == nil {
			deviceID = m.DeviceID
		}
		if desc == "" {
			desc = strings.TrimSpace(m.Description)
		}
		protocol := strings.ToUpper(firstNonEmpty(m.Protocol, "TCP"))
		parts = append(parts, fmt.Sprintf("%s %d → %s:%d", protocol, m.ExternalPort, m.InternalIP, m.InternalPort))
	}

	body := fmt.Sprintf("El router abrió %s.", strings.Join(parts, " y "))
	title := fmt.Sprintf("Se abrió un puerto hacia %s", k.InternalIP)
	if desc != "" {
		body += fmt.Sprintf(" La pidió «%s».", desc)
		title = fmt.Sprintf("%s abrió un puerto en tu router", desc)
	}

	return Alert{
		Type: PortMappingAdded, Severity: sev, At: at,
		Title:    title,
		Body:     body,
		DeviceID: deviceID,
		Network:  network,
		// Sin la hora: un aviso por equipo y programa, para siempre.
		DedupKey: fmt.Sprintf("%s|%s|%s|%s|%s|%d", PortMappingAdded, network, k.InternalIP, k.Program, k.Protocol, k.InternalPort),
	}
}

// ---------------------------------------------------------------------------
// RISKY_SERVICE (ver docs/agente-api.md, "Seguridad v1.2" > Alertas nuevas)
// ---------------------------------------------------------------------------

// RiskyDeviceInput es un equipo del anillo interior tal como lo necesita
// EvaluateNewRiskyServices: su identidad (para el id de la alerta y para
// redactar) y los puertos abiertos de ESTE escaneo (ver history.KnownDevice.Last.OpenPorts).
type RiskyDeviceInput struct {
	DeviceID  int64
	Name      string // ver DisplayName: "" cae a IP al redactar (ver firstNonEmpty)
	IP        string
	OpenPorts []int
}

// RiskyPortSeen identifica un (equipo, puerto) que ya se evaluó como
// servicio de riesgo al menos una vez — lo que agent/store persiste para
// siempre (nunca se borra) en risky_service_seen.
type RiskyPortSeen struct {
	DeviceID int64
	Port     int
}

// EvaluateNewRiskyServices decide qué RISKY_SERVICE corresponden: un puerto
// de riesgo medio o alto (ver core.LookupPort) que un equipo tiene abierto
// AHORA y que no está en alreadySeen (ver docs/plan.md, Fase 5:
// "Guarda los ya vistos por equipo para no repetir aunque el puerto
// aparezca y desaparezca entre escaneos" — a diferencia de
// PORT_MAPPING_ADDED, esta SÍ es "para siempre": un puerto ya avisado no
// vuelve a avisar aunque se cierre y se reabra).
//
// Devuelve, además de las alertas, TODOS los pares (equipo, puerto)
// riesgosos-y-nuevos de este escaneo en newlySeen — alertados o no (ver
// hasBaseline) — para que quien llama los persista de una sola pasada sin
// repetir la misma búsqueda en el catálogo de puertos. hasBaseline=false
// (igual que EvaluateNewPortMappings) deja newlySeen con lo que haya, pero
// newAlerts siempre vacío: solo evita la tormenta del primer arranque (el
// PC de la casa, por ejemplo, ya tenía 139/445/554/3389 abiertos antes de
// que existiera esta regla).
func EvaluateNewRiskyServices(devices []RiskyDeviceInput, alreadySeen map[int64]map[int]bool, hasBaseline bool, network string, at time.Time, settings Settings) (newAlerts []Alert, newlySeen []RiskyPortSeen) {
	for _, d := range devices {
		ports := append([]int(nil), d.OpenPorts...)
		sort.Ints(ports)
		last := -1
		for _, port := range ports {
			if port == last {
				continue // OpenPorts no debería traer duplicados, pero no cuesta nada ser robusto
			}
			last = port
			entry, ok := core.LookupPort(port)
			if !ok || (entry.Risk != "medium" && entry.Risk != "high") {
				continue
			}
			if alreadySeen[d.DeviceID][port] {
				continue
			}
			newlySeen = append(newlySeen, RiskyPortSeen{DeviceID: d.DeviceID, Port: port})
			if hasBaseline && settings.RiskyService {
				newAlerts = append(newAlerts, riskyServiceAlert(d, port, entry, network, at))
			}
		}
	}
	return newAlerts, newlySeen
}

func riskyServiceAlert(d RiskyDeviceInput, port int, entry core.PortEntry, network string, at time.Time) Alert {
	sev := Medium
	if entry.Risk == "high" {
		sev = High
	}
	who := firstNonEmpty(d.Name, d.IP)
	id := d.DeviceID
	return Alert{
		Type: RiskyService, Severity: sev, At: at,
		Title:    fmt.Sprintf("%s tiene abierto %s (%d)", who, entry.Name, port),
		Body:     entry.Why,
		DeviceID: &id,
		Network:  network,
		DedupKey: dedupKey(RiskyService, fmt.Sprintf("%d|%d", d.DeviceID, port), at),
	}
}

// ---------------------------------------------------------------------------
// INTERNET_DOWN / INTERNET_RESTORED (Fase 7, ver docs/agente-api.md,
// "Internet (v1.4)" > Caídas). A diferencia de las demás reglas de este
// archivo, el evento de origen no es un escaneo de core.Scan sino una
// Outage de agent/internet.OutageDetector (ver cmd/vigia-agent/internet.go,
// que llama a estas dos funciones cuando el detector abre/cierra una
// caída). DeviceID siempre nil: es un evento de la red, no de un equipo
// (mismo criterio que AGENT_BLIND/AGENT_RECOVERED).
// ---------------------------------------------------------------------------

// EvaluateInternetDown redacta INTERNET_DOWN al confirmarse una caída
// (ver agent/internet.OutageDetector.Feed: quien llama solo invoca esto
// cuando el detector DEVOLVIÓ una Outage recién abierta, no en cada
// medición). nil si el tipo está apagado en settings.
func EvaluateInternetDown(blame internet.Blame, network string, startedAt time.Time, settings Settings) *Alert {
	if !settings.InternetDown {
		return nil
	}
	a := Alert{
		Type: InternetDown, Severity: High, At: startedAt,
		Title:    "Se cayó el internet",
		Body:     "Se cayó el internet · " + blameDownText(blame),
		Network:  network,
		DedupKey: dedupKey(InternetDown, network, startedAt),
	}
	return &a
}

// EvaluateInternetRestored redacta INTERNET_RESTORED al cerrarse una caída
// (ver agent/internet.OutageDetector.Feed: quien llama solo invoca esto
// cuando el detector devolvió una Outage con End ya puesto). startedAt
// identifica la MISMA caída que abrió el INTERNET_DOWN correspondiente
// (mismo instante: ver DedupKey) para que un reintento de esta evaluación
// nunca duplique el aviso de regreso, igual que con la caída. nil si el
// tipo está apagado en settings.
func EvaluateInternetRestored(blame internet.Blame, network string, startedAt, endedAt time.Time, settings Settings) *Alert {
	if !settings.InternetRestored {
		return nil
	}
	dur := endedAt.Sub(startedAt)
	a := Alert{
		Type: InternetRestored, Severity: Low, At: endedAt,
		Title:    "Volvió el internet",
		Body:     fmt.Sprintf("Volvió el internet · Estuvo caído %s. Culpable: %s.", formatDuration(dur), blameRestoredText(blame)),
		Network:  network,
		DedupKey: dedupKey(InternetRestored, network, startedAt),
	}
	return &a
}

// blameDownText: el cuerpo de INTERNET_DOWN según el culpable (ver
// docs/plan.md, Fase 7: 'Se cayó el internet · El problema parece estar en
// tu proveedor (tu router y el módem responden).').
func blameDownText(b internet.Blame) string {
	switch b {
	case internet.BlameLAN:
		return "El problema parece estar en tu router: no está respondiendo."
	case internet.BlameModem:
		return "El problema parece estar en el módem (tu router responde, pero el módem no)."
	case internet.BlameISP:
		return "El problema parece estar en tu proveedor (tu router y el módem responden)."
	default:
		return "No se pudo determinar en qué parte de la red está el problema."
	}
}

// blameRestoredText: a quién se le echa la culpa en el cuerpo de
// INTERNET_RESTORED (español de México, breve: ya no hace falta explicar,
// solo nombrar).
func blameRestoredText(b internet.Blame) string {
	switch b {
	case internet.BlameLAN:
		return "tu router"
	case internet.BlameModem:
		return "tu módem"
	case internet.BlameISP:
		return "tu proveedor"
	default:
		return "no se sabe"
	}
}

// formatDuration da una duración en español de México, redondeada al
// minuto (o a los segundos si duró menos de uno): "3 min", "1 h 12 min",
// "45 s".
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		secs := int(d.Round(time.Second).Seconds())
		return fmt.Sprintf("%d s", secs)
	}
	d = d.Round(time.Minute)
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	if h == 0 {
		return fmt.Sprintf("%d min", m)
	}
	if m == 0 {
		return fmt.Sprintf("%d h", h)
	}
	return fmt.Sprintf("%d h %d min", h, m)
}
