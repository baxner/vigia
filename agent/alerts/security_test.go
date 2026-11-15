package alerts

import (
	"strings"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/core"
)

const innerGateway = "192.168.0.4"

// --- EvaluateScan: el cuerpo de NEW_DEVICE cambia solo por Ring. ---

func TestEvaluateScan_NewDevice_AnilloExterior_CuerpoDistinto(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at := t0.Add(time.Hour)
	events := []EventInput{{Event: history.DeviceEvent{DeviceID: 1, Type: history.FirstSeen, At: at}, Device: mkDevice(1, "192.168.100.3", false)}}

	got := EvaluateScan(events, testNetwork, t0, DefaultSettings(), RingOuter)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	want := "192.168.100.3 se conectó por primera vez a la red de delante de tu router (la del módem)."
	if got[0].Body != want {
		t.Fatalf("body = %q, want %q", got[0].Body, want)
	}

	gotInner := EvaluateScan(events, testNetwork, t0, DefaultSettings(), RingInner)
	if want := "192.168.100.3 se conectó por primera vez."; gotInner[0].Body != want {
		t.Fatalf("body (interior) = %q, want %q", gotInner[0].Body, want)
	}
}

// --- ROUTER_CHANGED ---

func TestEvaluateRouterChanged_CambioRealGeneraAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	a := EvaluateRouterChanged("02:00:00:00:00:04", "aa:bb:cc:dd:ee:ff", testNetwork, at, DefaultSettings())
	if a == nil {
		t.Fatal("a = nil, want ROUTER_CHANGED")
	}
	if a.Type != RouterChanged || a.Severity != High {
		t.Fatalf("type/severity = %s/%s, want ROUTER_CHANGED/high", a.Type, a.Severity)
	}
	if a.Network != testNetwork {
		t.Fatalf("network = %q, want %q", a.Network, testNetwork)
	}
	if a.DedupKey == "" {
		t.Fatal("DedupKey vacío")
	}
}

func TestEvaluateRouterChanged_SinCambio_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	if a := EvaluateRouterChanged("02:00:00:00:00:04", "02:00:00:00:00:04", testNetwork, at, DefaultSettings()); a != nil {
		t.Fatalf("a = %+v, want nil (misma MAC)", a)
	}
}

func TestEvaluateRouterChanged_SinGuardadaONuevaDesconocida_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	settings := DefaultSettings()
	if a := EvaluateRouterChanged("", "aa:bb:cc:dd:ee:ff", testNetwork, at, settings); a != nil {
		t.Fatalf("sin anterior: a = %+v, want nil (primera vez, nada que comparar)", a)
	}
	if a := EvaluateRouterChanged("02:00:00:00:00:04", "", testNetwork, at, settings); a != nil {
		t.Fatalf("sin actual: a = %+v, want nil (ARP no resolvió esta vez)", a)
	}
}

func TestEvaluateRouterChanged_Apagado_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	settings := DefaultSettings()
	settings.RouterChanged = false
	if a := EvaluateRouterChanged("02:00:00:00:00:04", "aa:bb:cc:dd:ee:ff", testNetwork, at, settings); a != nil {
		t.Fatalf("a = %+v, want nil (ROUTER_CHANGED apagado)", a)
	}
}

// --- ARP_CONFLICT ---

func mkScanned(ip, mac string) core.Device { return core.Device{IP: ip, MAC: mac} }

func TestEvaluateARPConflict_OtroEquipoConocidoResponde_GeneraAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	scanned := []core.Device{mkScanned(innerGateway, "aa:bb:cc:dd:ee:01"), mkScanned("192.168.0.55", "aa:bb:cc:dd:ee:01")}
	mac := "aa:bb:cc:dd:ee:01"
	known := []history.KnownDevice{{ID: 5, LastIP: "192.168.0.55", MAC: &mac}}

	a := EvaluateARPConflict(innerGateway, scanned, known, testNetwork, at, DefaultSettings())
	if a == nil {
		t.Fatal("a = nil, want ARP_CONFLICT")
	}
	if a.Type != ArpConflict || a.Severity != High {
		t.Fatalf("type/severity = %s/%s, want ARP_CONFLICT/high", a.Type, a.Severity)
	}
}

func TestEvaluateARPConflict_LaMACEsDelPropioRouter_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	scanned := []core.Device{mkScanned(innerGateway, "02:00:00:00:00:04")}
	mac := "02:00:00:00:00:04"
	// El propio router, conocido con LastIP == gatewayIP: no es "otro equipo".
	known := []history.KnownDevice{{ID: 1, LastIP: innerGateway, MAC: &mac}}

	if a := EvaluateARPConflict(innerGateway, scanned, known, testNetwork, at, DefaultSettings()); a != nil {
		t.Fatalf("a = %+v, want nil (es el router mismo)", a)
	}
}

func TestEvaluateARPConflict_SinMACEnEsteEscaneo_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	mac := "aa:bb:cc:dd:ee:01"
	known := []history.KnownDevice{{ID: 5, LastIP: "192.168.0.55", MAC: &mac}}
	// El gateway no respondió en este escaneo (no está en `scanned`).
	if a := EvaluateARPConflict(innerGateway, nil, known, testNetwork, at, DefaultSettings()); a != nil {
		t.Fatalf("a = %+v, want nil (sin MAC que comparar)", a)
	}
}

func TestEvaluateARPConflict_MACDesconocida_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	scanned := []core.Device{mkScanned(innerGateway, "ff:ff:ff:ff:ff:ff")}
	// Ningún equipo conocido tiene esa MAC: podría ser un router nuevo
	// (ROUTER_CHANGED se encarga de eso), no un conflicto.
	if a := EvaluateARPConflict(innerGateway, scanned, nil, testNetwork, at, DefaultSettings()); a != nil {
		t.Fatalf("a = %+v, want nil (MAC no asociada a ningún equipo conocido)", a)
	}
}

func TestEvaluateARPConflict_DedupPorDia(t *testing.T) {
	mac := "aa:bb:cc:dd:ee:01"
	known := []history.KnownDevice{{ID: 5, LastIP: "192.168.0.55", MAC: &mac}}
	scanned := []core.Device{mkScanned(innerGateway, mac)}

	at1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at2 := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC) // mismo día
	at3 := time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC)  // día siguiente

	a1 := EvaluateARPConflict(innerGateway, scanned, known, testNetwork, at1, DefaultSettings())
	a2 := EvaluateARPConflict(innerGateway, scanned, known, testNetwork, at2, DefaultSettings())
	a3 := EvaluateARPConflict(innerGateway, scanned, known, testNetwork, at3, DefaultSettings())
	if a1 == nil || a2 == nil || a3 == nil {
		t.Fatal("las tres evaluaciones deberían generar una alerta (el dedup lo hace la UNIQUE de agent/store, no esta función)")
	}
	if a1.DedupKey != a2.DedupKey {
		t.Fatalf("mismo día: DedupKey distinta: %q != %q", a1.DedupKey, a2.DedupKey)
	}
	if a1.DedupKey == a3.DedupKey {
		t.Fatalf("día siguiente: DedupKey debería ser distinta, fue igual: %q", a1.DedupKey)
	}
}

func TestEvaluateARPConflict_Apagado_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	mac := "aa:bb:cc:dd:ee:01"
	known := []history.KnownDevice{{ID: 5, LastIP: "192.168.0.55", MAC: &mac}}
	scanned := []core.Device{mkScanned(innerGateway, mac)}
	settings := DefaultSettings()
	settings.ArpConflict = false
	if a := EvaluateARPConflict(innerGateway, scanned, known, testNetwork, at, settings); a != nil {
		t.Fatalf("a = %+v, want nil (ARP_CONFLICT apagado)", a)
	}
}

// --- PORT_MAPPING_ADDED ---

func mkMapping(proto string, extPort int, ip string, intPort int) core.PortMapping {
	return core.PortMapping{Protocol: proto, ExternalPort: extPort, InternalIP: ip, InternalPort: intPort, Source: "upnp"}
}

func TestEvaluateNewPortMappings_Nueva_GeneraAlertaMedium(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	current := []core.PortMapping{mkMapping("TCP", 8080, "192.168.0.55", 80)} // 80 = HTTP, risk "low"

	got := EvaluateNewPortMappings(nil, current, true, testNetwork, at, DefaultSettings())
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Type != PortMappingAdded || got[0].Severity != Medium {
		t.Fatalf("type/severity = %s/%s, want PORT_MAPPING_ADDED/medium (riesgo bajo -> medium por defecto)", got[0].Type, got[0].Severity)
	}
}

func TestEvaluateNewPortMappings_PuertoInteriorDeRiesgoAlto_SeveridadHigh(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	current := []core.PortMapping{mkMapping("TCP", 2323, "192.168.0.55", 23)} // 23 = Telnet, risk "high"

	got := EvaluateNewPortMappings(nil, current, true, testNetwork, at, DefaultSettings())
	if len(got) != 1 || got[0].Severity != High {
		t.Fatalf("got = %+v, want 1 alerta high (puerto interior de riesgo alto)", got)
	}
}

func seen(ms ...core.PortMapping) map[PortMappingNotifyKey]bool {
	out := make(map[PortMappingNotifyKey]bool)
	for _, m := range ms {
		out[PortMappingNotifyKeyOf(m)] = true
	}
	return out
}

func TestEvaluateNewPortMappings_YaVista_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m := mkMapping("TCP", 8080, "192.168.0.55", 80)
	if got := EvaluateNewPortMappings(seen(m), []core.PortMapping{m}, true, testNetwork, at, DefaultSettings()); len(got) != 0 {
		t.Fatalf("got = %+v, want 0 (ya se había visto)", got)
	}
}

func TestEvaluateNewPortMappings_ReaparecidaORenovada_NoVuelveAAvisar(t *testing.T) {
	// Caso real: Tailscale renueva sus redirecciones por
	// NAT-PMP y cada renovación avisaba otra vez. Una vez vista, no se repite.
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	tailscale := mkMapping("UDP", 41641, "192.168.0.48", 41641)
	if got := EvaluateNewPortMappings(seen(tailscale), []core.PortMapping{tailscale}, true, testNetwork, at, DefaultSettings()); len(got) != 0 {
		t.Fatalf("got = %+v, want 0 (reapareció tras renovarse)", got)
	}
}

func TestEvaluateNewPortMappings_MismoEquipoOtroPuertoExterior_NoAvisa(t *testing.T) {
	// AnyDesk pide un puerto exterior distinto en cada sesión hacia el mismo 7070.
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	ayer := mkMapping("TCP", 43957, "192.168.0.63", 7070)
	hoy := mkMapping("TCP", 44559, "192.168.0.63", 7070)
	if got := EvaluateNewPortMappings(seen(ayer), []core.PortMapping{hoy}, true, testNetwork, at, DefaultSettings()); len(got) != 0 {
		t.Fatalf("got = %+v, want 0 (mismo equipo y puerto interno)", got)
	}
}

func TestEvaluateNewPortMappings_UnaSolaAlertaPorEquipoYPuerto(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	// Dos redirecciones nuevas hacia el mismo equipo y puerto (TCP y otra externa): un solo aviso.
	current := []core.PortMapping{mkMapping("TCP", 5000, "192.168.0.70", 80), mkMapping("TCP", 5001, "192.168.0.70", 80)}
	got := EvaluateNewPortMappings(nil, current, true, testNetwork, at, DefaultSettings())
	if len(got) != 1 {
		t.Fatalf("got = %d alertas, want 1", len(got))
	}
	if strings.Contains(got[0].DedupKey, "2026") {
		t.Fatalf("DedupKey = %q: no debe llevar la hora (una sola alerta para siempre)", got[0].DedupKey)
	}
}

func TestEvaluateNewPortMappings_SinLineaBase_NuncaAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	current := []core.PortMapping{
		mkMapping("TCP", 139, "192.168.0.55", 139),
		mkMapping("TCP", 445, "192.168.0.55", 445),
		mkMapping("UDP", 3389, "192.168.0.55", 3389),
	}
	if got := EvaluateNewPortMappings(nil, current, false, testNetwork, at, DefaultSettings()); len(got) != 0 {
		t.Fatalf("got = %+v, want 0 (sin línea base: no se arriesga una tormenta de alertas)", got)
	}
}

func TestEvaluateNewPortMappings_Apagado_NoAlerta(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	current := []core.PortMapping{mkMapping("TCP", 8080, "192.168.0.55", 80)}
	settings := DefaultSettings()
	settings.PortMappingAdded = false
	if got := EvaluateNewPortMappings(nil, current, true, testNetwork, at, settings); len(got) != 0 {
		t.Fatalf("got = %+v, want 0 (PORT_MAPPING_ADDED apagado)", got)
	}
}

func TestEvaluateNewPortMappings_ProtocoloVacioCaeATCP(t *testing.T) {
	// Un router raro que mande Protocol="" no debería producir una clave
	// distinta de la misma redirección con Protocol="TCP" explícito.
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	previous := []core.PortMapping{mkMapping("TCP", 8080, "192.168.0.55", 80)}
	current := []core.PortMapping{{ExternalPort: 8080, InternalIP: "192.168.0.55", InternalPort: 80}}
	if got := EvaluateNewPortMappings(seen(previous...), current, true, testNetwork, at, DefaultSettings()); len(got) != 0 {
		t.Fatalf("got = %+v, want 0 (misma redirección, Protocol vacío = TCP)", got)
	}
}

// --- PORT_MAPPING_ADDED: agrupar por equipo + programa (Fase 5.1) ---

func mkMappingDesc(proto string, extPort int, ip string, intPort int, desc string) core.PortMapping {
	m := mkMapping(proto, extPort, ip, intPort)
	m.Description = desc
	return m
}

// Caso real: HyperTorrent (en el teléfono, 192.168.0.105)
// pide un puerto interior DISTINTO en cada sesión. Con la descripción, debe
// avisar una sola vez por equipo+programa, sin importar que el puerto
// interior cambie.
func TestEvaluateNewPortMappings_HyperTorrent_CambiaDePuertoInterior_UnaSolaVezPorPrograma(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	ayer := mkMappingDesc("TCP", 51413, "192.168.0.105", 54321, "HyperTorrent 3.2.5")
	hoy := mkMappingDesc("TCP", 55102, "192.168.0.105", 60123, "HyperTorrent 3.2.5")

	if got := EvaluateNewPortMappings(seen(ayer), []core.PortMapping{hoy}, true, testNetwork, at, DefaultSettings()); len(got) != 0 {
		t.Fatalf("got = %+v, want 0 (mismo equipo y programa, puerto interior distinto)", got)
	}
}

// TCP y UDP de la misma sesión (HyperTorrent las pide juntas): un solo aviso.
func TestEvaluateNewPortMappings_TCPYUDPDeLaMismaPeticion_UnSoloAviso(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	tcp := mkMappingDesc("TCP", 51413, "192.168.0.105", 51413, "HyperTorrent 3.2.5")
	udp := mkMappingDesc("UDP", 51413, "192.168.0.105", 51413, "HyperTorrent 3.2.5")

	got := EvaluateNewPortMappings(nil, []core.PortMapping{tcp, udp}, true, testNetwork, at, DefaultSettings())
	if len(got) != 1 {
		t.Fatalf("got = %d alertas, want 1 (TCP+UDP de la misma petición)", len(got))
	}
	if !strings.Contains(got[0].Body, "TCP 51413") || !strings.Contains(got[0].Body, "UDP 51413") {
		t.Fatalf("body = %q, quería que mencionara TCP y UDP", got[0].Body)
	}
}

// Dos equipos distintos con el MISMO programa: un aviso por equipo (la
// clave incluye la IP interior, no solo el programa).
func TestEvaluateNewPortMappings_MismoProgramaDistintoEquipo_AvisaLosDos(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	tel1 := mkMappingDesc("TCP", 1001, "192.168.0.10", 2001, "AnyDesk")
	tel2 := mkMappingDesc("TCP", 1002, "192.168.0.11", 2002, "AnyDesk")

	got := EvaluateNewPortMappings(nil, []core.PortMapping{tel1, tel2}, true, testNetwork, at, DefaultSettings())
	if len(got) != 2 {
		t.Fatalf("got = %d alertas, want 2 (equipos distintos, mismo programa)", len(got))
	}
}

// Sin descripción: sigue avisando por protocolo + IP + puerto interior,
// como antes de la Fase 5.1.
func TestEvaluateNewPortMappings_SinDescripcion_ComoAntes(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m := mkMapping("TCP", 8080, "192.168.0.55", 80) // sin Description
	got := EvaluateNewPortMappings(nil, []core.PortMapping{m}, true, testNetwork, at, DefaultSettings())
	if len(got) != 1 {
		t.Fatalf("got = %d, want 1", len(got))
	}
	// Ya visto exactamente ese (protocolo, IP, puerto interior): no repite.
	if got := EvaluateNewPortMappings(seen(m), []core.PortMapping{m}, true, testNetwork, at, DefaultSettings()); len(got) != 0 {
		t.Fatalf("got = %+v, want 0 (ya visto, sin descripción)", got)
	}
}

func TestNormalizeDescription(t *testing.T) {
	cases := []struct{ in, want string }{
		{"HyperTorrent 3.2.5", "hypertorrent"},
		{"NAT-PMP 844780211", "nat-pmp"},
		{"tailscale-portmap", "tailscale-portmap"},
		{"AnyDesk", "anydesk"},
		{"DemonwarePortMapping", "demonwareportmapping"},
		{"", ""},
		{"   ", ""},
		{"12345", ""}, // solo un número suelto: no identifica nada
	}
	for _, c := range cases {
		if got := NormalizeDescription(c.in); got != c.want {
			t.Errorf("NormalizeDescription(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- RISKY_SERVICE ---

func TestEvaluateNewRiskyServices_PuertoRiesgoso_GeneraAlertaYRegistraVisto(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	devices := []RiskyDeviceInput{{DeviceID: 12, Name: "DESKTOP-ABC1234", IP: "192.168.0.55", OpenPorts: []int{80, 445}}}

	gotAlerts, gotSeen := EvaluateNewRiskyServices(devices, nil, true, testNetwork, at, DefaultSettings())
	if len(gotAlerts) != 1 {
		t.Fatalf("alerts = %+v, want 1 (solo 445 es de riesgo medio/alto; 80 es low)", gotAlerts)
	}
	a := gotAlerts[0]
	if a.Type != RiskyService || a.Severity != Medium {
		t.Fatalf("type/severity = %s/%s, want RISKY_SERVICE/medium", a.Type, a.Severity)
	}
	if a.DeviceID == nil || *a.DeviceID != 12 {
		t.Fatalf("deviceId = %v, want 12", a.DeviceID)
	}
	if len(gotSeen) != 1 || gotSeen[0] != (RiskyPortSeen{DeviceID: 12, Port: 445}) {
		t.Fatalf("newlySeen = %+v, want [{12 445}]", gotSeen)
	}
}

func TestEvaluateNewRiskyServices_RiesgoAlto_SeveridadHigh(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	devices := []RiskyDeviceInput{{DeviceID: 1, IP: "192.168.0.2", OpenPorts: []int{23}}}
	got, _ := EvaluateNewRiskyServices(devices, nil, true, testNetwork, at, DefaultSettings())
	if len(got) != 1 || got[0].Severity != High {
		t.Fatalf("got = %+v, want 1 alerta high (Telnet)", got)
	}
}

func TestEvaluateNewRiskyServices_YaVisto_NoRepite(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	devices := []RiskyDeviceInput{{DeviceID: 12, IP: "192.168.0.55", OpenPorts: []int{445}}}
	alreadySeen := map[int64]map[int]bool{12: {445: true}}

	gotAlerts, gotSeen := EvaluateNewRiskyServices(devices, alreadySeen, true, testNetwork, at, DefaultSettings())
	if len(gotAlerts) != 0 || len(gotSeen) != 0 {
		t.Fatalf("alerts=%+v seen=%+v, want ambos vacíos (ya se había avisado)", gotAlerts, gotSeen)
	}
}

func TestEvaluateNewRiskyServices_DesapareceYReaparece_NoRepite(t *testing.T) {
	// A diferencia de PORT_MAPPING_ADDED: un puerto que se cierra y se
	// vuelve a abrir NO debe volver a avisar (ver alreadySeen).
	at1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at2 := at1.Add(10 * time.Minute)
	devices := []RiskyDeviceInput{{DeviceID: 12, IP: "192.168.0.55", OpenPorts: []int{445}}}

	_, seen1 := EvaluateNewRiskyServices(devices, nil, true, testNetwork, at1, DefaultSettings())
	alreadySeen := map[int64]map[int]bool{}
	for _, s := range seen1 {
		if alreadySeen[s.DeviceID] == nil {
			alreadySeen[s.DeviceID] = map[int]bool{}
		}
		alreadySeen[s.DeviceID][s.Port] = true
	}
	// "desaparece" (un escaneo sin el puerto) no se modela aquí porque
	// alreadySeen es permanente: lo que importa es que, al "reaparecer"
	// con el mismo alreadySeen, no vuelva a alertar.
	gotAlerts, gotSeen := EvaluateNewRiskyServices(devices, alreadySeen, true, testNetwork, at2, DefaultSettings())
	if len(gotAlerts) != 0 || len(gotSeen) != 0 {
		t.Fatalf("alerts=%+v seen=%+v, want ambos vacíos (reapareció, pero ya estaba 'visto para siempre')", gotAlerts, gotSeen)
	}
}

func TestEvaluateNewRiskyServices_SinLineaBase_RegistraSinAlertar(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	// El PC de la casa real (ver docs/plan.md): 139, 445, 554, 3389.
	devices := []RiskyDeviceInput{{DeviceID: 1, IP: "192.168.0.55", OpenPorts: []int{139, 445, 554, 3389}}}

	gotAlerts, gotSeen := EvaluateNewRiskyServices(devices, nil, false, testNetwork, at, DefaultSettings())
	if len(gotAlerts) != 0 {
		t.Fatalf("alerts = %+v, want 0 (sin línea base: no se arriesga una tormenta)", gotAlerts)
	}
	if len(gotSeen) != 4 {
		t.Fatalf("newlySeen = %+v, want 4 (los 4 puertos riesgosos del PC, para que la próxima vez ya no sean 'nuevos')", gotSeen)
	}
}

func TestEvaluateNewRiskyServices_PuertoNoRiesgoso_Ignorado(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	devices := []RiskyDeviceInput{{DeviceID: 1, IP: "192.168.0.2", OpenPorts: []int{80, 443}}} // low/none
	gotAlerts, gotSeen := EvaluateNewRiskyServices(devices, nil, true, testNetwork, at, DefaultSettings())
	if len(gotAlerts) != 0 || len(gotSeen) != 0 {
		t.Fatalf("alerts=%+v seen=%+v, want ambos vacíos (80/443 no son riesgo medio/alto)", gotAlerts, gotSeen)
	}
}

func TestEvaluateNewRiskyServices_Apagado_NoAlertaPeroSiRegistra(t *testing.T) {
	// El ajuste solo debe silenciar la ALERTA, no la memoria: si se
	// reactiva el ajuste más adelante, no queremos una tormenta retroactiva.
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	devices := []RiskyDeviceInput{{DeviceID: 1, IP: "192.168.0.2", OpenPorts: []int{445}}}
	settings := DefaultSettings()
	settings.RiskyService = false

	gotAlerts, gotSeen := EvaluateNewRiskyServices(devices, nil, true, testNetwork, at, settings)
	if len(gotAlerts) != 0 {
		t.Fatalf("alerts = %+v, want 0 (RISKY_SERVICE apagado)", gotAlerts)
	}
	if len(gotSeen) != 1 {
		t.Fatalf("newlySeen = %+v, want 1 (se registra igual, aunque no avise)", gotSeen)
	}
}

// --- Settings.Enabled: las 4 claves nuevas ---

func TestSettings_Enabled_ClavesNuevas(t *testing.T) {
	s := DefaultSettings()
	for _, typ := range []Type{RouterChanged, ArpConflict, PortMappingAdded, RiskyService} {
		if !s.Enabled(typ) {
			t.Errorf("Enabled(%s) = false, want true (DefaultSettings)", typ)
		}
	}
	s.RouterChanged = false
	if s.Enabled(RouterChanged) {
		t.Error("Enabled(RouterChanged) = true tras apagarlo")
	}
}
