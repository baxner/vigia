package core

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// Informe de seguridad (Fase 5, ver docs/agente-api.md "Seguridad v1.2").
// PURO: nada de este archivo toca la red, el reloj del sistema ni disco —
// BuildSecurityReport recibe todo lo que necesita (SecurityInput) y la hora
// (now), y devuelve el mismo SecurityReport sin importar quién lo llame
// (agente o app/mobile): así se puede probar con datos armados a mano.
//
// *** HUECO DE CONTRATO (ver el reporte final del subagente) ***
// docs/agente-api.md dice "OuterRing.devices: [KnownDevice]", pero
// KnownDevice (history.KnownDevice, con firstSeen/lastSeen/custom...) es un
// tipo del AGENTE y de Android: core no lo conoce ni debería (quedaría con
// un import en el sentido equivocado, agent -> core es la dirección buena).
// Aquí OuterRing.Devices usa la misma forma reducida que ya pide el
// contrato para el "devices" de nivel superior de SecurityInput/Mobile.securityReport
// ({"id","name","online","device"}: ver SecurityDevice). Quien construya el
// JSON final de /api/v1/security (el agente, fuera de esta fase) decide si
// eso le basta o si prefiere reemplazar "outerRing.devices" por el
// KnownDevice completo después de llamar a BuildSecurityReport — esta
// función solo necesita poder CONTAR y NOMBRAR esos equipos.

// Severity son las cuatro severidades del contrato.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
	SeverityInfo   Severity = "info"
)

// FindingKind son los tipos de hallazgo del contrato (ver
// docs/agente-api.md, "Tipos de hallazgo").
type FindingKind string

const (
	KindNoInbound        FindingKind = "NO_INBOUND"
	KindUPnPEnabled      FindingKind = "UPNP_ENABLED"
	KindPortMapping      FindingKind = "PORT_MAPPING"
	KindRiskyService     FindingKind = "RISKY_SERVICE"
	KindUnknownDevices   FindingKind = "UNKNOWN_DEVICES"
	KindOuterRingDevices FindingKind = "OUTER_RING_DEVICES"
	KindRouterChanged    FindingKind = "ROUTER_CHANGED"
	KindARPConflict      FindingKind = "ARP_CONFLICT"
	KindWifiOpen         FindingKind = "WIFI_OPEN"
	KindWifiWEP          FindingKind = "WIFI_WEP"
	KindWifiWPA2         FindingKind = "WIFI_WPA2"
	KindWifiWPS          FindingKind = "WIFI_WPS"
)

// riskyServiceCap: tope a la suma de puntos de TODOS los RISKY_SERVICE al
// calcular el puntaje (cada Finding.Points individual queda sin tocar, para
// que siga diciendo la verdad sobre ESE hallazgo; el tope solo limita cuánto
// resta la categoría completa — ver computeScore).
const riskyServiceCap = 45

// Finding es un hallazgo del informe (ver docs/agente-api.md). IP/Port/
// DeviceID son punteros a propósito: el contrato pide "null si no aplica",
// no el campo ausente ni un cero-valor ambiguo (IP "" también sería
// válido como string vacío, Port 0 es un puerto de verdad en teoría).
type Finding struct {
	ID       string      `json:"id"`
	Kind     FindingKind `json:"kind"`
	Severity Severity    `json:"severity"`
	Points   int         `json:"points"`
	Title    string      `json:"title"`
	Detail   string      `json:"detail"`
	Action   string      `json:"action"`
	DeviceID *int64      `json:"deviceId"`
	IP       *string     `json:"ip"`
	Port     *int        `json:"port"`
	// Acknowledged/AcknowledgedAt: "ya lo sé, es a propósito" (v1.3, ver
	// docs/agente-api.md "Hallazgos reconocidos"). applyAcknowledgements
	// los llena por ID DESPUÉS de construir el hallazgo; nunca restan del
	// puntaje (ver computeScore) y van al final de Findings (ver
	// sortFindings).
	Acknowledged   bool       `json:"acknowledged"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt"`
}

// Acknowledgement es "ya lo sé, es a propósito" sobre UN hallazgo (ver
// docs/agente-api.md, "Hallazgos reconocidos v1.3"). FindingID es el id
// ESTABLE del hallazgo (ver findingWho), no su posición en Findings: se
// puede reconocer un hallazgo que ya no aparece en el informe actual (p.
// ej. un puerto que se cerró) sin que eso sea un error, simplemente no
// hace nada hasta que vuelva a aparecer.
type Acknowledgement struct {
	FindingID string    `json:"findingId"`
	At        time.Time `json:"at"`
	Note      *string   `json:"note"`
}

// SecurityDevice es la forma reducida de un dispositivo que usan
// SecurityInput y OuterRing: lo que BuildSecurityReport necesita (si está en
// línea, su nombre para mostrar y el core.Device con OpenPorts/Identity),
// no el historial completo que solo tiene el agente.
type SecurityDevice struct {
	ID     *int64 `json:"id"`
	Name   string `json:"name"`
	Online bool   `json:"online"`
	Device Device `json:"device"`
}

// WifiInfo: lo que solo la app sabe (el agente va por cable). Security vacío
// o un valor fuera de los cuatro que reconocemos (open/wep/wpa2 — "wpa" y
// "wpa3"/"wpa2-wpa3" no tienen hallazgo propio en el contrato de la Fase 5)
// simplemente no agrega ningún WIFI_* de cifrado; WPS es independiente.
type WifiInfo struct {
	SSID     string `json:"ssid"`
	Security string `json:"security"` // open|wep|wpa|wpa2|wpa3|wpa2-wpa3
	WPS      bool   `json:"wps"`
}

// Alert es la vista mínima de una alerta ya guardada que BuildSecurityReport
// necesita (ver docs/agente-api.md, "Alertas"): solo el tipo y la hora, para
// decidir si hubo un ROUTER_CHANGED/ARP_CONFLICT en las últimas 24 horas. La
// forma completa (id, severidad, título, cuerpo...) vive en
// agent/store.Alert — decodifica igual porque encoding/json ignora los
// campos de más al llenar este struct más chico.
type Alert struct {
	Type string    `json:"type"`
	At   time.Time `json:"at"`
}

// OuterRing es el anillo exterior (la red de delante del router) tal como lo
// necesita el informe: unos cuantos datos de identificación y sus equipos
// (ver el aviso de HUECO DE CONTRATO al inicio de este archivo sobre la
// forma de Devices).
type OuterRing struct {
	Network    string           `json:"network"`
	Subnet     string           `json:"subnet"`
	Gateway    string           `json:"gateway"`
	LastScanAt *time.Time       `json:"lastScanAt"`
	Devices    []SecurityDevice `json:"devices"`
}

// SecurityInput es todo lo que BuildSecurityReport necesita para calcular el
// informe (ver Mobile.securityReport en docs/agente-api.md).
type SecurityInput struct {
	Devices      []SecurityDevice `json:"devices"`
	Router       *RouterInfo      `json:"router"`
	OuterRing    *OuterRing       `json:"outerRing"`
	Wifi         *WifiInfo        `json:"wifi"`
	RecentAlerts []Alert          `json:"recentAlerts"`
	// Acknowledged: los "ya lo sé, es a propósito" vigentes (v1.3, ver
	// docs/agente-api.md "Hallazgos reconocidos"). El agente los guarda
	// compartidos entre teléfonos; la app sin agente, en el teléfono.
	Acknowledged []Acknowledgement `json:"acknowledged"`
}

// SecurityReport es el informe completo.
type SecurityReport struct {
	GeneratedAt time.Time   `json:"generatedAt"`
	Score       int         `json:"score"`
	Findings    []Finding   `json:"findings"`
	Router      *RouterInfo `json:"router"`
	OuterRing   *OuterRing  `json:"outerRing"`
}

// BuildSecurityReport calcula el informe de seguridad. PURO: now es la
// "hora actual" (para ROUTER_CHANGED/ARP_CONFLICT, "últimas 24 horas");
// nunca llama a time.Now() por su cuenta, para poder probarse con un reloj
// fijo.
func BuildSecurityReport(in SecurityInput, now time.Time) SecurityReport {
	// Listas vacías como [] y no null en el JSON: más fácil para quien lo lea
	// (la app, un webhook, Home Assistant).
	if in.Router != nil && in.Router.PortMappings == nil {
		r := *in.Router
		r.PortMappings = []PortMapping{}
		in.Router = &r
	}
	reach := "unknown"
	if in.Router != nil && in.Router.Reach != "" {
		reach = in.Router.Reach
	}

	var findings []Finding

	if in.Router != nil && reach == "outer_ring" {
		findings = append(findings, noInboundFinding(in.Router, in.OuterRing))
	}
	if in.Router != nil && in.Router.UPnP {
		findings = append(findings, upnpEnabledFinding(reach))
	}
	if in.Router != nil {
		for _, pm := range in.Router.PortMappings {
			findings = append(findings, portMappingFinding(pm, reach, nameByIP(in.Devices)[pm.InternalIP], ringHint(in.Router, in.OuterRing)))
		}
	}
	findings = append(findings, riskyServiceFindings(in.Devices)...)
	if f, ok := unknownDevicesFinding(in.Devices); ok {
		findings = append(findings, f)
	}
	if in.OuterRing != nil {
		findings = append(findings, outerRingFinding(*in.OuterRing))
	}
	if hasRecentAlert(in.RecentAlerts, "ROUTER_CHANGED", now) {
		findings = append(findings, routerChangedFinding())
	}
	if hasRecentAlert(in.RecentAlerts, "ARP_CONFLICT", now) {
		findings = append(findings, arpConflictFinding())
	}
	findings = append(findings, wifiFindings(in.Wifi)...)

	applyAcknowledgements(findings, in.Acknowledged)
	score := computeScore(findings)
	sortFindings(findings)
	if findings == nil {
		findings = []Finding{} // "[]" en JSON, nunca "null"
	}

	return SecurityReport{
		GeneratedAt: now.UTC(),
		Score:       score,
		Findings:    findings,
		Router:      in.Router,
		OuterRing:   in.OuterRing,
	}
}

// computeScore: 100 menos la suma de los puntos de cada hallazgo NO
// reconocido (ver docs/agente-api.md "Hallazgos reconocidos v1.3": "no
// restan"), con el aporte de TODOS los RISKY_SERVICE juntos (los no
// reconocidos) topado en riskyServiceCap (ver su comentario) y el total
// nunca por debajo de 0.
func computeScore(findings []Finding) int {
	total, risky := 0, 0
	for _, f := range findings {
		if f.Acknowledged {
			continue
		}
		if f.Kind == KindRiskyService {
			risky += f.Points
			continue
		}
		total += f.Points
	}
	if risky > riskyServiceCap {
		risky = riskyServiceCap
	}
	total += risky
	score := 100 - total
	if score < 0 {
		score = 0
	}
	return score
}

// applyAcknowledgements marca en findings los que el usuario reconoció como
// intencionales (ver docs/agente-api.md, "Hallazgos reconocidos v1.3"), por
// FindingID — el id ESTABLE del hallazgo, no su posición: reconocer un
// hallazgo que ahora no aparece en el informe (p. ej. un puerto que ya se
// cerró) simplemente no hace nada, no es un error. Muta findings in-place
// (slice ya construido por BuildSecurityReport, antes de sortFindings).
func applyAcknowledgements(findings []Finding, acks []Acknowledgement) {
	if len(acks) == 0 {
		return
	}
	byID := make(map[string]Acknowledgement, len(acks))
	for _, a := range acks {
		byID[a.FindingID] = a
	}
	for i := range findings {
		a, ok := byID[findings[i].ID]
		if !ok {
			continue
		}
		findings[i].Acknowledged = true
		at := a.At
		findings[i].AcknowledgedAt = &at
	}
}

func severityRank(s Severity) int {
	switch s {
	case SeverityHigh:
		return 0
	case SeverityMedium:
		return 1
	case SeverityLow:
		return 2
	default: // SeverityInfo y cualquier otra cosa: al final
		return 3
	}
}

// sortFindings: "orden: severidad y luego puntos" (ver docs/plan.md,
// Fase 5), pero los reconocidos (ver docs/agente-api.md, "Hallazgos
// reconocidos v1.3": "van al final") siempre después de los que no lo
// están, sin importar su severidad/puntos. Estable: entre hallazgos con el
// mismo orden de reconocimiento/severidad/puntos, se conserva el orden en
// que BuildSecurityReport los fue agregando.
func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Acknowledged != findings[j].Acknowledged {
			return !findings[i].Acknowledged
		}
		if ri, rj := severityRank(findings[i].Severity), severityRank(findings[j].Severity); ri != rj {
			return ri < rj
		}
		return findings[i].Points > findings[j].Points
	})
}

// ---------------------------------------------------------------------------
// NO_INBOUND
// ---------------------------------------------------------------------------

func noInboundFinding(router *RouterInfo, outer *OuterRing) Finding {
	// El motor solo sabe que la salida del router es una IP privada o CGNAT:
	// no puede afirmar qué hace el equipo de delante (si redirige o no).
	exit := "una IP privada"
	if router != nil && router.ExternalIP != "" {
		exit = fmt.Sprintf("una IP privada (%s)", router.ExternalIP)
	}
	return Finding{
		ID:       string(KindNoInbound),
		Kind:     KindNoInbound,
		Severity: SeverityInfo,
		Points:   0,
		Title:    "Tu router no está expuesto directo a internet",
		Detail: fmt.Sprintf(
			"Su salida es %s: lo que redirige solo lo ven los equipos de la red de delante (%s).",
			exit, ringHint(router, outer)),
		Action: "Nada que hacer. Solo quedaría algo visible desde internet si el equipo de delante (el módem) también redirigiera puertos hacia tu router; con CGNAT del proveedor, ni así.",
	}
}

// ringHint: "192.168.100.x" a partir del subnet del anillo exterior (si lo
// tenemos) o de la IP externa del router — para que el texto de NO_INBOUND
// no quede con un ejemplo fijo cuando la red real de alguien no use
// 192.168.100.0/24.
func ringHint(router *RouterInfo, outer *OuterRing) string {
	if outer != nil && outer.Subnet != "" {
		if _, n, err := net.ParseCIDR(outer.Subnet); err == nil {
			if v4 := n.IP.To4(); v4 != nil {
				return fmt.Sprintf("%d.%d.%d.x", v4[0], v4[1], v4[2])
			}
		}
	}
	if router != nil && router.ExternalIP != "" {
		if v4 := net.ParseIP(router.ExternalIP).To4(); v4 != nil {
			return fmt.Sprintf("%d.%d.%d.x", v4[0], v4[1], v4[2])
		}
	}
	return "la red de delante de tu router"
}

// ---------------------------------------------------------------------------
// UPNP_ENABLED
// ---------------------------------------------------------------------------

func upnpEnabledFinding(reach string) Finding {
	sev, pts := SeverityLow, 5
	where := "hacia la red de delante de tu router"
	if reach == "internet" {
		sev, pts = SeverityMedium, 10
		where = "hacia internet"
	}
	return Finding{
		ID:       string(KindUPnPEnabled),
		Kind:     KindUPnPEnabled,
		Severity: sev,
		Points:   pts,
		Title:    "UPnP está activado en tu router",
		Detail: fmt.Sprintf(
			"El router tiene UPnP activado: cualquier equipo de tu red le puede pedir que abra un puerto %s, sin que nadie lo apruebe.",
			where),
		Action: "Si ningún equipo de tu red necesita abrir puertos solo (consolas, algunos juegos o apps de videollamada), desactiva UPnP en el router; si algo deja de funcionar, vuelve a activarlo.",
	}
}

// ---------------------------------------------------------------------------
// PORT_MAPPING
// ---------------------------------------------------------------------------

// portMappingDetail: la regla, quién la pidió (la descripción que mandó el
// programa por UPnP, p. ej. "AnyDesk") y desde dónde se ve.
func portMappingDetail(pm PortMapping, reach, ring string) string {
	d := fmt.Sprintf("%s %d → %s:%d", firstNonEmpty(pm.Protocol, "TCP"), pm.ExternalPort, pm.InternalIP, pm.InternalPort)
	if desc := strings.TrimSpace(pm.Description); desc != "" {
		d += fmt.Sprintf(", la pidió «%s»", desc)
	}
	switch reach {
	case "internet":
		d += ". Se ve desde internet."
	case "outer_ring":
		d += fmt.Sprintf(". Solo la ven los equipos de la red de delante (%s).", ring)
	default:
		d += "."
	}
	return d
}

func portMappingAction(pm PortMapping) string {
	if pm.Source == "upnp" {
		return "La abrió por su cuenta un programa de ese equipo (por UPnP). Si no lo reconoces, desinstálalo; si nada en tu casa necesita abrir puertos, desactiva UPnP en el router."
	}
	return "Si ya no la usas, bórrala desde el panel del router."
}

// nameByIP: nombre de cada equipo conocido por su IP, para redactar.
func nameByIP(devices []SecurityDevice) map[string]string {
	out := make(map[string]string, len(devices))
	for _, d := range devices {
		if d.Name != "" {
			out[d.Device.IP] = d.Name
		}
	}
	return out
}

func portMappingFinding(pm PortMapping, reach, name, ring string) Finding {
	sev, pts := SeverityLow, 3
	if entry, ok := LookupPort(pm.InternalPort); ok && isRiskyLevel(entry.Risk) {
		sev, pts = SeverityMedium, 8
	}
	if reach == "internet" {
		pts *= 2
	}
	ip := pm.InternalIP
	port := pm.ExternalPort
	protocol := strings.ToUpper(firstNonEmpty(pm.Protocol, "TCP"))
	return Finding{
		// Por puerto INTERIOR + protocolo, no por el exterior (ver
		// docs/agente-api.md v1.3, ejemplo "PORT_MAPPING|dev:12|7070/TCP"):
		// el exterior cambia de sesión en sesión (AnyDesk) y un id que
		// dependiera de él perdería el reconocimiento en cada renovación.
		ID:       fmt.Sprintf("%s|%s|%d/%s", KindPortMapping, findingWho(pm.DeviceID, pm.InternalIP), pm.InternalPort, protocol),
		Kind:     KindPortMapping,
		Severity: sev,
		Points:   pts,
		Title:    fmt.Sprintf("El router abre el puerto %d hacia %s", pm.ExternalPort, firstNonEmpty(name, pm.InternalIP)),
		Detail:   portMappingDetail(pm, reach, ring),
		Action:   portMappingAction(pm),
		IP:       &ip,
		Port:     &port,
		DeviceID: pm.DeviceID,
	}
}

func isRiskyLevel(risk string) bool {
	return risk == "medium" || risk == "high"
}

// findingWho: la mitad "a quién" del id ESTABLE de un hallazgo por equipo
// (ver docs/agente-api.md, "Hallazgos reconocidos v1.3"): el id del equipo
// si se conoce ("dev:7"), para que el reconocimiento no se pierda si el
// equipo cambia de IP; sin id (la IP sola, o la app sin agente), la IP
// como antes de la Fase 5.1.
func findingWho(deviceID *int64, ip string) string {
	if deviceID != nil {
		return fmt.Sprintf("dev:%d", *deviceID)
	}
	return ip
}

// ---------------------------------------------------------------------------
// RISKY_SERVICE
// ---------------------------------------------------------------------------

func riskyServiceFindings(devices []SecurityDevice) []Finding {
	var out []Finding
	for _, d := range devices {
		if !d.Online {
			continue
		}
		ports := append([]int(nil), d.Device.OpenPorts...)
		sort.Ints(ports)
		seen := map[int]bool{}
		for _, port := range ports {
			if seen[port] {
				continue
			}
			seen[port] = true
			entry, ok := LookupPort(port)
			if !ok || !isRiskyLevel(entry.Risk) {
				continue
			}
			sev, pts := SeverityMedium, 8
			if entry.Risk == "high" {
				sev, pts = SeverityHigh, 15
			}
			ip := d.Device.IP
			p := port
			who := firstNonEmpty(d.Name, ip)
			out = append(out, Finding{
				ID:       fmt.Sprintf("%s|%s|%d", KindRiskyService, findingWho(d.ID, ip), port),
				Kind:     KindRiskyService,
				Severity: sev,
				Points:   pts,
				Title:    fmt.Sprintf("%s tiene abierto %s (%d)", who, entry.Name, port),
				Detail:   entry.Why,
				Action:   entry.Action,
				DeviceID: d.ID,
				IP:       &ip,
				Port:     &p,
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// UNKNOWN_DEVICES
// ---------------------------------------------------------------------------

func unknownDevicesFinding(devices []SecurityDevice) (Finding, bool) {
	n := 0
	for _, d := range devices {
		if d.Online && d.Name == "" && (d.Device.Identity.Type == "" || d.Device.Identity.Type == "unknown") {
			n++
		}
	}
	if n == 0 {
		return Finding{}, false
	}
	pts := n * 2
	if pts > 10 {
		pts = 10
	}
	word := "equipo conectado no se pudo identificar ni tiene un nombre guardado"
	if n != 1 {
		word = "equipos conectados no se pudieron identificar ni tienen un nombre guardado"
	}
	return Finding{
		ID:       string(KindUnknownDevices),
		Kind:     KindUnknownDevices,
		Severity: SeverityLow,
		Points:   pts,
		Title:    "Hay equipos sin identificar en tu red",
		Detail:   fmt.Sprintf("%d %s.", n, word),
		Action:   "Revísalos en la lista de tu red; si no reconoces alguno, considera cambiar la contraseña de tu Wi-Fi.",
	}, true
}

// ---------------------------------------------------------------------------
// OUTER_RING_DEVICES
// ---------------------------------------------------------------------------

func outerRingFinding(outer OuterRing) Finding {
	n := len(outer.Devices)
	if n == 0 {
		return Finding{
			ID:       string(KindOuterRingDevices),
			Kind:     KindOuterRingDevices,
			Severity: SeverityInfo,
			Points:   0,
			Title:    "Equipos en la red de delante de tu router",
			Detail:   "No hay otros equipos visibles en la red de delante de tu router.",
			Action:   "No necesitas hacer nada por esto; es solo información.",
		}
	}
	labels := make([]string, 0, n)
	for _, d := range outer.Devices {
		label := d.Device.IP
		if d.Name != "" {
			label = fmt.Sprintf("%s (%s)", d.Name, d.Device.IP)
		}
		labels = append(labels, label)
	}
	word := "equipo"
	if n != 1 {
		word = "equipos"
	}
	return Finding{
		ID:       string(KindOuterRingDevices),
		Kind:     KindOuterRingDevices,
		Severity: SeverityInfo,
		Points:   0,
		Title:    "Equipos en la red de delante de tu router",
		Detail: fmt.Sprintf("Hay %d %s en la red del módem (la de delante de tu router): %s.",
			n, word, strings.Join(labels, ", ")),
		Action: "Es normal ver aquí el módem o la ONT. Si hay algo que no reconoces, revisa quién está conectado a la Wi-Fi o a los puertos del módem: esos equipos ven lo que tu router redirige.",
	}
}

// ---------------------------------------------------------------------------
// ROUTER_CHANGED / ARP_CONFLICT
// ---------------------------------------------------------------------------

// hasRecentAlert dice si alertas trae alguna de tipo typ con At dentro de las
// últimas 24 horas contadas desde now (now - 24h, exclusivo; now inclusive).
func hasRecentAlert(alerts []Alert, typ string, now time.Time) bool {
	cutoff := now.Add(-24 * time.Hour)
	for _, a := range alerts {
		if a.Type == typ && a.At.After(cutoff) {
			return true
		}
	}
	return false
}

func routerChangedFinding() Finding {
	return Finding{
		ID:       string(KindRouterChanged),
		Kind:     KindRouterChanged,
		Severity: SeverityHigh,
		Points:   30,
		Title:    "La puerta de enlace de tu red cambió",
		Detail:   "La MAC de tu router cambió en las últimas 24 horas. Puede ser que lo hayas reemplazado o reiniciado con otra interfaz, pero también puede ser alguien haciéndose pasar por él.",
		Action:   "Si no cambiaste de router, revisa tu red con cuidado y considera cambiar la contraseña de tu Wi-Fi.",
	}
}

func arpConflictFinding() Finding {
	return Finding{
		ID:       string(KindARPConflict),
		Kind:     KindARPConflict,
		Severity: SeverityHigh,
		Points:   30,
		Title:    "Un equipo respondió como si fuera el router",
		Detail:   "Un equipo de tu red contestó con la misma IP que tu router, pero con otra MAC. Puede ser un problema de red, pero también alguien tratando de interceptar tu tráfico.",
		Action:   "Desconecta los equipos que no reconozcas y, si vuelve a pasar, cambia la contraseña de tu Wi-Fi.",
	}
}

// ---------------------------------------------------------------------------
// WIFI_*
// ---------------------------------------------------------------------------

// wifiFindings: solo WIFI_OPEN/WIFI_WEP/WIFI_WPA2/WIFI_WPS tienen regla en
// el contrato de la Fase 5 (ver docs/plan.md): un Security fuera de
// esos (p. ej. "wpa" sin más, "wpa3", "wpa2-wpa3") no agrega ningún hallazgo
// de cifrado — ver el aviso al inicio del archivo.
func wifiFindings(w *WifiInfo) []Finding {
	if w == nil {
		return nil
	}
	var out []Finding
	switch strings.ToLower(w.Security) {
	case "open":
		out = append(out, Finding{
			ID: string(KindWifiOpen), Kind: KindWifiOpen, Severity: SeverityHigh, Points: 30,
			Title:  "Tu Wi-Fi no tiene contraseña",
			Detail: fmt.Sprintf("La red Wi-Fi %q está abierta: cualquiera cerca de tu casa se puede conectar sin pedir nada.", w.SSID),
			Action: "Ponle una contraseña (WPA2 o WPA3) desde la configuración del router.",
		})
	case "wep":
		out = append(out, Finding{
			ID: string(KindWifiWEP), Kind: KindWifiWEP, Severity: SeverityHigh, Points: 25,
			Title:  "Tu Wi-Fi usa un cifrado viejo e inseguro (WEP)",
			Detail: fmt.Sprintf("La red Wi-Fi %q usa WEP, un cifrado roto desde hace años: se puede descifrar en minutos.", w.SSID),
			Action: "Cambia la seguridad de tu Wi-Fi a WPA2 o WPA3 desde la configuración del router.",
		})
	case "wpa2":
		out = append(out, Finding{
			ID: string(KindWifiWPA2), Kind: KindWifiWPA2, Severity: SeverityInfo, Points: 0,
			Title:  "Tu Wi-Fi usa WPA2",
			Detail: fmt.Sprintf("La red Wi-Fi %q usa WPA2, un cifrado todavía seguro si tiene una contraseña fuerte.", w.SSID),
			Action: "No necesitas cambiar nada por esto; si además puedes activar WPA3, es un paso más de seguridad, no obligatorio.",
		})
	}
	if w.WPS {
		out = append(out, Finding{
			ID: string(KindWifiWPS), Kind: KindWifiWPS, Severity: SeverityMedium, Points: 10,
			Title:  "WPS está activado en tu Wi-Fi",
			Detail: "El router acepta conectar equipos por WPS, un método con un PIN que se puede adivinar por fuerza bruta.",
			Action: "Desactiva WPS en la configuración del router; conecta equipos nuevos escribiendo la contraseña.",
		})
	}
	return out
}
