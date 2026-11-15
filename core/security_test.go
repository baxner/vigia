package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func i64(v int64) *int64 { return &v }

// fixedNow: el reloj fijo de todas las pruebas de este archivo (ver
// docs/plan.md, "Pruebas obligatorias": nada que dependa de la
// fecha real).
var fixedNow = time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC)

// ===========================================================================
// Caso realista de red doméstica (Fase 5):
//   - Router 192.168.0.4, salida 192.168.100.2, reach outer_ring, una
//     redirección UPnP (RDP, 3389 -> 192.168.0.55:3389: así lo usa de
//     verdad, para entrar por RDP desde la red de la ONT).
//   - DESKTOP-ABC1234 (192.168.0.55) con 135, 139, 445, 554 y 3389 abiertos.
//   - 2 equipos sin identificar.
//   - Un anillo exterior con la ONT 192.168.100.1.
// ===========================================================================

func realisticSecurityInput() SecurityInput {
	router := &RouterInfo{
		IP:         "192.168.0.4",
		MAC:        "02:00:00:00:00:04",
		UPnP:       true,
		NATPMP:     true,
		ExternalIP: "192.168.100.2",
		Reach:      "outer_ring",
		PortMappings: []PortMapping{
			{Protocol: "TCP", ExternalPort: 3389, InternalIP: "192.168.0.55", InternalPort: 3389, Description: "RDP", Source: "upnp"},
		},
	}
	outer := &OuterRing{
		Network: "192.168.100.1|192.168.100.0/24",
		Subnet:  "192.168.100.0/24",
		Gateway: "192.168.100.1",
		Devices: []SecurityDevice{
			{ID: nil, Name: "", Online: true, Device: Device{IP: "192.168.100.1", Identity: Identity{Type: "router"}}},
		},
	}
	return SecurityInput{
		Devices: []SecurityDevice{
			{
				ID: i64(12), Name: "DESKTOP-ABC1234", Online: true,
				Device: Device{
					IP:        "192.168.0.55",
					OpenPorts: []int{135, 139, 445, 554, 3389},
					Identity:  Identity{Type: "computer", Confidence: 95},
				},
			},
			{ID: i64(20), Name: "", Online: true, Device: Device{IP: "192.168.0.21", Identity: Identity{Type: "unknown"}}},
			{ID: i64(21), Name: "", Online: true, Device: Device{IP: "192.168.0.22", Identity: Identity{Type: "unknown"}}},
		},
		Router:    router,
		OuterRing: outer,
	}
}

func TestBuildSecurityReport_RedReal(t *testing.T) {
	report := BuildSecurityReport(realisticSecurityInput(), fixedNow)

	if report.GeneratedAt != fixedNow {
		t.Errorf("GeneratedAt = %v, want %v", report.GeneratedAt, fixedNow)
	}
	if report.Score != 51 {
		t.Errorf("Score = %d, want 51 (100 - [8 port_mapping + 4*8 risky + 5 upnp + 4 unknown + 0 + 0])", report.Score)
	}
	if len(report.Findings) != 9 {
		t.Fatalf("len(Findings) = %d, want 9: %+v", len(report.Findings), report.Findings)
	}

	byID := map[string]Finding{}
	for _, f := range report.Findings {
		if _, dup := byID[f.ID]; dup {
			t.Errorf("id de hallazgo repetido: %s", f.ID)
		}
		byID[f.ID] = f
	}

	cases := []struct {
		id       string
		kind     FindingKind
		severity Severity
		points   int
	}{
		{"NO_INBOUND", KindNoInbound, SeverityInfo, 0},
		{"UPNP_ENABLED", KindUPnPEnabled, SeverityLow, 5},
		// Sin DeviceID en el RouterInfo.PortMappings de este fixture: cae a
		// la IP (ver findingWho). Por puerto INTERIOR + protocolo (v1.3).
		{"PORT_MAPPING|192.168.0.55|3389/TCP", KindPortMapping, SeverityMedium, 8},
		// El equipo SÍ tiene ID (i64(12)): el id del hallazgo usa "dev:12",
		// no la IP (ver docs/agente-api.md, "Hallazgos reconocidos v1.3").
		{"RISKY_SERVICE|dev:12|139", KindRiskyService, SeverityMedium, 8},
		{"RISKY_SERVICE|dev:12|445", KindRiskyService, SeverityMedium, 8},
		{"RISKY_SERVICE|dev:12|554", KindRiskyService, SeverityMedium, 8},
		{"RISKY_SERVICE|dev:12|3389", KindRiskyService, SeverityMedium, 8},
		{"UNKNOWN_DEVICES", KindUnknownDevices, SeverityLow, 4},
		{"OUTER_RING_DEVICES", KindOuterRingDevices, SeverityInfo, 0},
	}
	for _, c := range cases {
		f, ok := byID[c.id]
		if !ok {
			t.Errorf("falta el hallazgo %q", c.id)
			continue
		}
		if f.Kind != c.kind || f.Severity != c.severity || f.Points != c.points {
			t.Errorf("%s = {kind=%s sev=%s pts=%d}, want {%s %s %d}",
				c.id, f.Kind, f.Severity, f.Points, c.kind, c.severity, c.points)
		}
	}

	// No debe haber RISKY_SERVICE para el 135 (RPC de Windows, risk=low: no
	// cuenta como riesgo medio/alto).
	if _, ok := byID["RISKY_SERVICE|dev:12|135"]; ok {
		t.Error("RISKY_SERVICE para el puerto 135 no debía existir (su riesgo en el catálogo es low)")
	}

	// Orden: severidad y luego puntos (ver sortFindings).
	assertFindingsOrdered(t, report.Findings)

	// Textos clave que pide el encargo literalmente.
	pm := byID["PORT_MAPPING|192.168.0.55|3389/TCP"]
	if !strings.Contains(pm.Detail, "192.168.0.55:3389") {
		t.Errorf("PORT_MAPPING.Detail = %q, quería que mencionara 192.168.0.55:3389", pm.Detail)
	}
	risky445 := byID["RISKY_SERVICE|dev:12|445"]
	wantTitle := "DESKTOP-ABC1234 tiene abierto SMB (445)"
	if risky445.Title != wantTitle {
		t.Errorf("RISKY_SERVICE(445).Title = %q, want %q", risky445.Title, wantTitle)
	}
	smbEntry, _ := LookupPort(445)
	if risky445.Detail != smbEntry.Why {
		t.Errorf("RISKY_SERVICE(445).Detail = %q, want el Why del catálogo (%q)", risky445.Detail, smbEntry.Why)
	}
	if risky445.Action != smbEntry.Action {
		t.Errorf("RISKY_SERVICE(445).Action = %q, want el Action del catálogo (%q)", risky445.Action, smbEntry.Action)
	}

	noInbound := byID["NO_INBOUND"]
	if !strings.Contains(noInbound.Detail, "192.168.100.x") {
		t.Errorf("NO_INBOUND.Detail = %q, quería que mencionara 192.168.100.x", noInbound.Detail)
	}

	outerF := byID["OUTER_RING_DEVICES"]
	if !strings.Contains(outerF.Detail, "192.168.100.1") || !strings.Contains(outerF.Detail, "1") {
		t.Errorf("OUTER_RING_DEVICES.Detail = %q, quería que mencionara 192.168.100.1 y el conteo", outerF.Detail)
	}

	// Router/OuterRing pasan sin cambios al informe (BuildSecurityReport es
	// puro: no reconstruye lo que ya le dieron).
	if report.Router == nil || report.Router.IP != "192.168.0.4" {
		t.Errorf("Router = %+v, want el mismo RouterInfo de la entrada", report.Router)
	}
	if report.OuterRing == nil || len(report.OuterRing.Devices) != 1 {
		t.Errorf("OuterRing = %+v, want el mismo OuterRing de la entrada", report.OuterRing)
	}
}

// assertFindingsOrdered comprueba "orden: severidad y luego puntos" sin
// fijar el orden exacto dentro de un empate (ver sortFindings): cada
// elemento debe tener severidad >= (en rango) al anterior, y si es igual,
// puntos <= al anterior.
func assertFindingsOrdered(t *testing.T, findings []Finding) {
	t.Helper()
	for i := 1; i < len(findings); i++ {
		prev, cur := findings[i-1], findings[i]
		rp, rc := severityRank(prev.Severity), severityRank(cur.Severity)
		if rp > rc {
			t.Errorf("orden roto en [%d]/[%d]: severidad %s vino antes de %s", i-1, i, prev.Severity, cur.Severity)
			continue
		}
		if rp == rc && prev.Points < cur.Points {
			t.Errorf("orden roto en [%d]/[%d]: puntos %d vinieron antes de %d (misma severidad %s)",
				i-1, i, prev.Points, cur.Points, prev.Severity)
		}
	}
}

// ===========================================================================
// Tope de RISKY_SERVICE (45): cada Finding guarda sus puntos reales, pero la
// categoría completa no resta más de 45 al puntaje.
// ===========================================================================

func TestBuildSecurityReport_RiskyServiceCap(t *testing.T) {
	// 4 puertos de riesgo "high" (15 pts cada uno) = 60 puntos nominales,
	// por encima del tope de 45.
	in := SecurityInput{
		Devices: []SecurityDevice{
			{ID: i64(1), Name: "equipo-riesgoso", Online: true, Device: Device{
				IP:        "192.168.0.99",
				OpenPorts: []int{23, 1723, 5555, 5900}, // telnet, VPN PPTP, ADB, VNC: los 4 "high" del catálogo
			}},
		},
	}
	report := BuildSecurityReport(in, fixedNow)

	var sumaReal int
	for _, f := range report.Findings {
		if f.Kind != KindRiskyService {
			t.Fatalf("hallazgo inesperado sin router/dispositivos adicionales: %+v", f)
		}
		if f.Points != 15 {
			t.Errorf("Finding %s .Points = %d, want 15 (el tope NO debe tocar el valor individual)", f.ID, f.Points)
		}
		if f.Severity != SeverityHigh {
			t.Errorf("Finding %s .Severity = %s, want high", f.ID, f.Severity)
		}
		sumaReal += f.Points
	}
	if sumaReal != 60 {
		t.Fatalf("suma nominal de RISKY_SERVICE = %d, want 60 (4 x 15)", sumaReal)
	}
	if report.Score != 55 { // 100 - min(60, 45) = 55
		t.Errorf("Score = %d, want 55 (100 - tope de 45, no - 60)", report.Score)
	}
}

// ===========================================================================
// PORT_MAPPING: puntos dobles solo cuando reach es "internet".
// ===========================================================================

func TestBuildSecurityReport_PortMappingDoublesOnInternet(t *testing.T) {
	mapping := PortMapping{Protocol: "TCP", ExternalPort: 8080, InternalIP: "192.168.0.10", InternalPort: 445}

	outer := BuildSecurityReport(SecurityInput{Router: &RouterInfo{Reach: "outer_ring", PortMappings: []PortMapping{mapping}}}, fixedNow)
	internet := BuildSecurityReport(SecurityInput{Router: &RouterInfo{Reach: "internet", PortMappings: []PortMapping{mapping}}}, fixedNow)
	unknown := BuildSecurityReport(SecurityInput{Router: &RouterInfo{Reach: "unknown", PortMappings: []PortMapping{mapping}}}, fixedNow)

	pmPoints := func(r SecurityReport) int {
		for _, f := range r.Findings {
			if f.Kind == KindPortMapping {
				return f.Points
			}
		}
		t.Fatal("no se encontró el hallazgo PORT_MAPPING")
		return -1
	}

	if got := pmPoints(outer); got != 8 {
		t.Errorf("PORT_MAPPING.Points (outer_ring) = %d, want 8 (445 es medium, sin duplicar)", got)
	}
	if got := pmPoints(internet); got != 16 {
		t.Errorf("PORT_MAPPING.Points (internet) = %d, want 16 (8 duplicado)", got)
	}
	if got := pmPoints(unknown); got != 8 {
		t.Errorf("PORT_MAPPING.Points (unknown) = %d, want 8 (solo se duplica con internet)", got)
	}
}

// ===========================================================================
// NO_INBOUND solo aparece con reach == "outer_ring".
// ===========================================================================

func TestBuildSecurityReport_NoInboundSoloOuterRing(t *testing.T) {
	hasNoInbound := func(reach string, router *RouterInfo) bool {
		r := BuildSecurityReport(SecurityInput{Router: router}, fixedNow)
		for _, f := range r.Findings {
			if f.Kind == KindNoInbound {
				return true
			}
		}
		return false
	}

	if !hasNoInbound("outer_ring", &RouterInfo{Reach: "outer_ring"}) {
		t.Error("reach=outer_ring: quería NO_INBOUND presente")
	}
	if hasNoInbound("internet", &RouterInfo{Reach: "internet"}) {
		t.Error("reach=internet: NO_INBOUND no debía aparecer (sí puede haber algo expuesto)")
	}
	if hasNoInbound("unknown", &RouterInfo{Reach: "unknown"}) {
		t.Error("reach=unknown: NO_INBOUND no debía aparecer (no sabemos si algo llega)")
	}
	if hasNoInbound("sin-router", nil) {
		t.Error("router=nil: NO_INBOUND no debía aparecer")
	}
}

// ===========================================================================
// Sin router: solo deben poder salir RISKY_SERVICE/UNKNOWN_DEVICES/
// OUTER_RING_DEVICES/ROUTER_CHANGED/ARP_CONFLICT/WIFI_*.
// ===========================================================================

func TestBuildSecurityReport_SinRouter(t *testing.T) {
	in := SecurityInput{
		Devices: []SecurityDevice{
			{ID: i64(1), Name: "", Online: true, Device: Device{IP: "192.168.0.50"}},
		},
	}
	report := BuildSecurityReport(in, fixedNow)
	for _, f := range report.Findings {
		switch f.Kind {
		case KindNoInbound, KindUPnPEnabled, KindPortMapping:
			t.Errorf("sin router no debía haber un hallazgo %s", f.Kind)
		}
	}
	if report.Router != nil {
		t.Errorf("Router = %+v, want nil", report.Router)
	}
	// El equipo sin nombre ni identificar sigue contando para UNKNOWN_DEVICES
	// aunque no haya router.
	found := false
	for _, f := range report.Findings {
		if f.Kind == KindUnknownDevices {
			found = true
		}
	}
	if !found {
		t.Error("quería UNKNOWN_DEVICES aunque no haya router")
	}
}

// ===========================================================================
// ROUTER_CHANGED / ARP_CONFLICT: ventana de 24 horas respecto de "now".
// ===========================================================================

func TestBuildSecurityReport_AlertasRecientes(t *testing.T) {
	cases := []struct {
		nombre string
		at     time.Time
		want   bool
	}{
		{"hace 1 hora", fixedNow.Add(-1 * time.Hour), true},
		{"hace 23h59", fixedNow.Add(-23*time.Hour - 59*time.Minute), true},
		{"justo ahora", fixedNow, true},
		{"hace 25 horas", fixedNow.Add(-25 * time.Hour), false},
		{"hace exactamente 24 horas", fixedNow.Add(-24 * time.Hour), false}, // límite exclusivo
	}
	for _, c := range cases {
		report := BuildSecurityReport(SecurityInput{RecentAlerts: []Alert{{Type: "ROUTER_CHANGED", At: c.at}}}, fixedNow)
		got := false
		for _, f := range report.Findings {
			if f.Kind == KindRouterChanged {
				got = true
			}
		}
		if got != c.want {
			t.Errorf("%s: ROUTER_CHANGED presente = %v, want %v", c.nombre, got, c.want)
		}
	}
}

func TestBuildSecurityReport_ARPConflict(t *testing.T) {
	report := BuildSecurityReport(SecurityInput{RecentAlerts: []Alert{{Type: "ARP_CONFLICT", At: fixedNow.Add(-time.Minute)}}}, fixedNow)
	var f *Finding
	for i := range report.Findings {
		if report.Findings[i].Kind == KindARPConflict {
			f = &report.Findings[i]
		}
	}
	if f == nil {
		t.Fatal("falta ARP_CONFLICT")
	}
	if f.Severity != SeverityHigh || f.Points != 30 {
		t.Errorf("ARP_CONFLICT = {%s %d}, want {high 30}", f.Severity, f.Points)
	}
	// Un tipo de alerta que no es ninguno de los dos no debe disparar nada.
	report2 := BuildSecurityReport(SecurityInput{RecentAlerts: []Alert{{Type: "NEW_DEVICE", At: fixedNow}}}, fixedNow)
	for _, f := range report2.Findings {
		if f.Kind == KindRouterChanged || f.Kind == KindARPConflict {
			t.Errorf("NEW_DEVICE no debía disparar %s", f.Kind)
		}
	}
}

// ===========================================================================
// WIFI_*
// ===========================================================================

func TestBuildSecurityReport_WifiOpen(t *testing.T) {
	report := BuildSecurityReport(SecurityInput{Wifi: &WifiInfo{SSID: "MiRedAbierta", Security: "open"}}, fixedNow)
	f := onlyFindingOfKind(t, report, KindWifiOpen)
	if f.Severity != SeverityHigh || f.Points != 30 {
		t.Errorf("WIFI_OPEN = {%s %d}, want {high 30}", f.Severity, f.Points)
	}
	if !strings.Contains(f.Detail, "MiRedAbierta") {
		t.Errorf("WIFI_OPEN.Detail = %q, quería que mencionara el SSID", f.Detail)
	}
}

func TestBuildSecurityReport_WifiWEP(t *testing.T) {
	report := BuildSecurityReport(SecurityInput{Wifi: &WifiInfo{SSID: "x", Security: "WEP"}}, fixedNow) // case-insensitive
	f := onlyFindingOfKind(t, report, KindWifiWEP)
	if f.Severity != SeverityHigh || f.Points != 25 {
		t.Errorf("WIFI_WEP = {%s %d}, want {high 25}", f.Severity, f.Points)
	}
}

func TestBuildSecurityReport_WifiWPA2(t *testing.T) {
	report := BuildSecurityReport(SecurityInput{Wifi: &WifiInfo{SSID: "x", Security: "wpa2"}}, fixedNow)
	f := onlyFindingOfKind(t, report, KindWifiWPA2)
	if f.Severity != SeverityInfo || f.Points != 0 {
		t.Errorf("WIFI_WPA2 = {%s %d}, want {info 0}", f.Severity, f.Points)
	}
}

func TestBuildSecurityReport_WifiWPS(t *testing.T) {
	report := BuildSecurityReport(SecurityInput{Wifi: &WifiInfo{SSID: "x", Security: "wpa2", WPS: true}}, fixedNow)
	var kinds []FindingKind
	for _, f := range report.Findings {
		kinds = append(kinds, f.Kind)
	}
	if !containsKind(kinds, KindWifiWPA2) || !containsKind(kinds, KindWifiWPS) {
		t.Errorf("Findings = %v, want WIFI_WPA2 y WIFI_WPS juntos", kinds)
	}
}

// TestBuildSecurityReport_WifiSinHallazgoDeCifradoReconocido: "wpa" (sin
// más), "wpa3" y "wpa2-wpa3" no tienen regla propia en el contrato de la
// Fase 5 (ver wifiFindings): no deben generar WIFI_OPEN/WEP/WPA2.
func TestBuildSecurityReport_WifiSinHallazgoDeCifradoReconocido(t *testing.T) {
	for _, sec := range []string{"wpa", "wpa3", "wpa2-wpa3", "", "algo-raro"} {
		report := BuildSecurityReport(SecurityInput{Wifi: &WifiInfo{SSID: "x", Security: sec}}, fixedNow)
		for _, f := range report.Findings {
			if f.Kind == KindWifiOpen || f.Kind == KindWifiWEP || f.Kind == KindWifiWPA2 {
				t.Errorf("security=%q: no esperaba %s", sec, f.Kind)
			}
		}
	}
}

func TestBuildSecurityReport_SinWifi(t *testing.T) {
	report := BuildSecurityReport(SecurityInput{}, fixedNow)
	if len(report.Findings) != 0 {
		t.Errorf("Findings = %+v, want vacío (sin router, sin dispositivos, sin wifi, sin alertas)", report.Findings)
	}
	if report.Score != 100 {
		t.Errorf("Score = %d, want 100 (nada que reste)", report.Score)
	}
}

// ===========================================================================
// UNKNOWN_DEVICES: tope de 10 puntos.
// ===========================================================================

func TestBuildSecurityReport_UnknownDevicesTope(t *testing.T) {
	var devices []SecurityDevice
	for i := 0; i < 8; i++ { // 8 x 2 = 16, por encima del tope de 10
		devices = append(devices, SecurityDevice{Online: true, Device: Device{IP: "192.168.0." + string(rune('1'+i))}})
	}
	report := BuildSecurityReport(SecurityInput{Devices: devices}, fixedNow)
	f := onlyFindingOfKind(t, report, KindUnknownDevices)
	if f.Points != 10 {
		t.Errorf("UNKNOWN_DEVICES.Points = %d, want 10 (tope)", f.Points)
	}
}

// Un equipo OFFLINE, o uno identificado, o uno con nombre, no cuenta para
// UNKNOWN_DEVICES.
func TestBuildSecurityReport_UnknownDevicesNoCuentaOfflineNiIdentificados(t *testing.T) {
	in := SecurityInput{Devices: []SecurityDevice{
		{Online: false, Device: Device{IP: "192.168.0.2"}},                                      // offline: no cuenta
		{Online: true, Name: "Mi laptop", Device: Device{IP: "192.168.0.3"}},                    // con nombre: no cuenta
		{Online: true, Device: Device{IP: "192.168.0.4", Identity: Identity{Type: "computer"}}}, // identificado: no cuenta
	}}
	report := BuildSecurityReport(in, fixedNow)
	for _, f := range report.Findings {
		if f.Kind == KindUnknownDevices {
			t.Errorf("no esperaba UNKNOWN_DEVICES: %+v", f)
		}
	}
}

// ===========================================================================
// El puntaje nunca baja de 0.
// ===========================================================================

func TestBuildSecurityReport_ScoreNuncaNegativo(t *testing.T) {
	in := SecurityInput{
		Router: &RouterInfo{Reach: "internet", UPnP: true, PortMappings: []PortMapping{
			{Protocol: "TCP", ExternalPort: 1, InternalIP: "192.168.0.2", InternalPort: 23}, // high (telnet) * 2 = 16
		}},
		Wifi:         &WifiInfo{Security: "open", WPS: true},                                                // 30 + 10
		RecentAlerts: []Alert{{Type: "ROUTER_CHANGED", At: fixedNow}, {Type: "ARP_CONFLICT", At: fixedNow}}, // 30 + 30
	}
	report := BuildSecurityReport(in, fixedNow)
	if report.Score != 0 {
		t.Errorf("Score = %d, want 0 (10 upnp_enabled + 16 port_mapping + 30 wifi_open + 10 wps + 30 router_changed + 30 arp_conflict = 126)", report.Score)
	}
}

// ===========================================================================
// ringHint
// ===========================================================================

func TestRingHint(t *testing.T) {
	if got := ringHint(&RouterInfo{ExternalIP: "192.168.100.2"}, nil); got != "192.168.100.x" {
		t.Errorf("ringHint(router sin outer) = %q, want %q", got, "192.168.100.x")
	}
	if got := ringHint(nil, &OuterRing{Subnet: "10.9.9.0/24"}); got != "10.9.9.x" {
		t.Errorf("ringHint(outer sin router) = %q, want %q", got, "10.9.9.x")
	}
	// El subnet del anillo exterior, si está, le gana a la IP externa del router.
	if got := ringHint(&RouterInfo{ExternalIP: "192.168.100.2"}, &OuterRing{Subnet: "172.30.5.0/24"}); got != "172.30.5.x" {
		t.Errorf("ringHint(ambos) = %q, want preferir el subnet del anillo exterior", got)
	}
	if got := ringHint(nil, nil); got == "" {
		t.Error("ringHint(nil, nil) = \"\", want un texto genérico no vacío")
	}
}

// ===========================================================================
// Finding en JSON: ip/port/deviceId deben salir "null", no omitirse ni
// quedar en "" / 0 (ver el comentario de Finding en security.go).
// ===========================================================================

func TestFindingJSON_CamposNull(t *testing.T) {
	f := Finding{ID: "NO_INBOUND", Kind: KindNoInbound, Severity: SeverityInfo}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"deviceId":null`, `"ip":null`, `"port":null`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON = %s, quería que contuviera %s", s, want)
		}
	}
}

func TestFindingJSON_CamposNoNull(t *testing.T) {
	ip := "192.168.0.55"
	port := 445
	id := int64(12)
	f := Finding{ID: "x", IP: &ip, Port: &port, DeviceID: &id}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"deviceId":12`, `"ip":"192.168.0.55"`, `"port":445`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON = %s, quería que contuviera %s", s, want)
		}
	}
}

// ===========================================================================
// Hallazgos reconocidos (v1.3, ver docs/agente-api.md).
// ===========================================================================

func TestFindingID_RiskyService_EstableAlCambiarIP(t *testing.T) {
	mk := func(ip string) SecurityInput {
		return SecurityInput{Devices: []SecurityDevice{{ID: i64(9), Online: true, Device: Device{IP: ip, OpenPorts: []int{445}}}}}
	}
	f1 := onlyFindingOfKind(t, BuildSecurityReport(mk("192.168.0.50"), fixedNow), KindRiskyService)
	f2 := onlyFindingOfKind(t, BuildSecurityReport(mk("192.168.0.99"), fixedNow), KindRiskyService)
	if f1.ID != f2.ID {
		t.Errorf("el id cambió con la IP: %q != %q", f1.ID, f2.ID)
	}
	if f1.ID != "RISKY_SERVICE|dev:9|445" {
		t.Errorf("ID = %q, want RISKY_SERVICE|dev:9|445", f1.ID)
	}
}

func TestFindingID_RiskyService_SinDeviceID_UsaIP(t *testing.T) {
	in := SecurityInput{Devices: []SecurityDevice{{ID: nil, Online: true, Device: Device{IP: "192.168.0.50", OpenPorts: []int{445}}}}}
	f := onlyFindingOfKind(t, BuildSecurityReport(in, fixedNow), KindRiskyService)
	if f.ID != "RISKY_SERVICE|192.168.0.50|445" {
		t.Errorf("ID = %q, want RISKY_SERVICE|192.168.0.50|445 (sin id de equipo, la app sin agente)", f.ID)
	}
}

func TestFindingID_PortMapping_ConDeviceID(t *testing.T) {
	devID := i64(12)
	pm := PortMapping{Protocol: "TCP", ExternalPort: 43957, InternalIP: "192.168.0.63", InternalPort: 7070, DeviceID: devID}
	f := portMappingFinding(pm, "outer_ring", "", "192.168.100.x")
	if f.ID != "PORT_MAPPING|dev:12|7070/TCP" {
		t.Errorf("ID = %q, want PORT_MAPPING|dev:12|7070/TCP (puerto INTERIOR, no el exterior que cambia de sesión)", f.ID)
	}
}

func TestBuildSecurityReport_Acknowledged_NoRestaPuntosPeroConservaPoints(t *testing.T) {
	in := SecurityInput{
		Devices: []SecurityDevice{{ID: i64(7), Name: "Cámara", Online: true, Device: Device{IP: "192.168.0.70", OpenPorts: []int{554}}}}, // RTSP, medium
	}
	before := BuildSecurityReport(in, fixedNow)
	f := onlyFindingOfKind(t, before, KindRiskyService)
	if before.Score != 100-f.Points {
		t.Fatalf("score sin reconocer = %d, want %d", before.Score, 100-f.Points)
	}

	ackAt := fixedNow.Add(-time.Hour)
	in.Acknowledged = []Acknowledgement{{FindingID: f.ID, At: ackAt}}
	after := BuildSecurityReport(in, fixedNow)
	g := onlyFindingOfKind(t, after, KindRiskyService)

	if after.Score != 100 {
		t.Errorf("score reconocido = %d, want 100 (un hallazgo reconocido no resta)", after.Score)
	}
	if !g.Acknowledged {
		t.Error("Acknowledged = false, want true")
	}
	if g.AcknowledgedAt == nil || !g.AcknowledgedAt.Equal(ackAt) {
		t.Errorf("AcknowledgedAt = %v, want %v", g.AcknowledgedAt, ackAt)
	}
	if g.Points != f.Points {
		t.Errorf("Points = %d, want que se conserve (%d) aunque no reste del puntaje", g.Points, f.Points)
	}
}

func TestBuildSecurityReport_Acknowledged_VaAlFinalAunqueSeaHigh(t *testing.T) {
	in := SecurityInput{
		Devices: []SecurityDevice{
			// Name puesto en ambos para que no disparen UNKNOWN_DEVICES además.
			{ID: i64(1), Name: "equipo-1", Online: true, Device: Device{IP: "192.168.0.2", OpenPorts: []int{23}}},  // Telnet, high
			{ID: i64(2), Name: "equipo-2", Online: true, Device: Device{IP: "192.168.0.3", OpenPorts: []int{554}}}, // RTSP, medium
		},
		Acknowledged: []Acknowledgement{{FindingID: "RISKY_SERVICE|dev:1|23", At: fixedNow}},
	}
	report := BuildSecurityReport(in, fixedNow)
	if len(report.Findings) != 2 {
		t.Fatalf("Findings = %+v, want 2", report.Findings)
	}
	last := report.Findings[len(report.Findings)-1]
	if !last.Acknowledged || last.Kind != KindRiskyService || last.Severity != SeverityHigh {
		t.Errorf("último hallazgo = %+v, want el high reconocido (va al final sin importar su severidad)", last)
	}
	if report.Findings[0].Acknowledged {
		t.Errorf("primer hallazgo = %+v, want el no reconocido primero", report.Findings[0])
	}
}

func TestBuildSecurityReport_Acknowledged_IDQueNoExisteNoHaceNada(t *testing.T) {
	in := SecurityInput{
		// Name puesto para que no dispare además UNKNOWN_DEVICES.
		Devices:      []SecurityDevice{{ID: i64(7), Name: "Cámara", Online: true, Device: Device{IP: "192.168.0.70", OpenPorts: []int{554}}}},
		Acknowledged: []Acknowledgement{{FindingID: "RISKY_SERVICE|dev:999|9999", At: fixedNow}},
	}
	report := BuildSecurityReport(in, fixedNow)
	f := onlyFindingOfKind(t, report, KindRiskyService)
	if f.Acknowledged {
		t.Error("Acknowledged = true, want false (el reconocimiento era de otro id)")
	}
	if report.Score != 100-f.Points {
		t.Errorf("score = %d, want %d (sin tocar, el reconocimiento no aplicó)", report.Score, 100-f.Points)
	}
}

// ===========================================================================
// helpers
// ===========================================================================

func onlyFindingOfKind(t *testing.T, report SecurityReport, kind FindingKind) Finding {
	t.Helper()
	for _, f := range report.Findings {
		if f.Kind == kind {
			return f
		}
	}
	t.Fatalf("no se encontró ningún hallazgo %s en %+v", kind, report.Findings)
	return Finding{}
}

func containsKind(kinds []FindingKind, k FindingKind) bool {
	for _, x := range kinds {
		if x == k {
			return true
		}
	}
	return false
}

func TestPortMappingFindingDiceQuienYDesdeDonde(t *testing.T) {
	pm := PortMapping{Protocol: "TCP", ExternalPort: 43957, InternalIP: "192.168.0.63", InternalPort: 7070,
		Description: "AnyDesk", Source: "upnp"}
	f := portMappingFinding(pm, "outer_ring", "", "192.168.100.x")

	if f.Title != "El router abre el puerto 43957 hacia 192.168.0.63" {
		t.Errorf("Title = %q", f.Title)
	}
	want := "TCP 43957 → 192.168.0.63:7070, la pidió «AnyDesk». Solo la ven los equipos de la red de delante (192.168.100.x)."
	if f.Detail != want {
		t.Errorf("Detail = %q, want %q", f.Detail, want)
	}
	if !strings.Contains(f.Action, "UPnP") {
		t.Errorf("Action de una redirección UPnP debería explicar que la abrió un programa: %q", f.Action)
	}

	named := portMappingFinding(pm, "internet", "Laptop de visita", "")
	if named.Title != "El router abre el puerto 43957 hacia Laptop de visita" || !strings.HasSuffix(named.Detail, "Se ve desde internet.") {
		t.Errorf("con nombre y reach internet: Title = %q, Detail = %q", named.Title, named.Detail)
	}
}
