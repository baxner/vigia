package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/core"
)

var testOuterNet = store.NetworkInfo{Key: "192.168.100.1|192.168.100.0/24", Subnet: "192.168.100.0/24", Gateway: "192.168.100.1", Role: "outer"}

func TestSecurity_SinTokenDa401(t *testing.T) {
	env := newTestEnv(t)
	resp := env.doJSON(http.MethodGet, "/api/v1/security", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// Sin ningún escaneo ni router todavía, el informe igual sale (ver
// docs/plan.md, Fase 5): score 100, listas vacías en vez de null.
func TestSecurity_SinNadaTodavia_InformeVacio(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	resp := env.doJSON(http.MethodGet, "/api/v1/security", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var report core.SecurityReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.Score != 100 {
		t.Fatalf("score = %d, want 100", report.Score)
	}
	if report.Router != nil {
		t.Fatalf("router = %+v, want nil", report.Router)
	}
	if report.OuterRing != nil {
		t.Fatalf("outerRing = %+v, want nil", report.OuterRing)
	}
	if report.Findings == nil {
		t.Fatal("findings = nil, want [] (nunca null)")
	}
}

// Con un equipo del anillo interior en línea con un puerto de riesgo y una
// redirección guardada por el router, el informe debe traer ambos
// hallazgos y el RouterInfo enriquecido con deviceId/firstSeen/lastSeen.
func TestSecurity_ConDispositivoYRouter(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	pc := env.seedDevice(testNet, core.Device{IP: "192.168.0.55", MAC: "aa:bb:cc:dd:ee:01", OpenPorts: []int{445}}, at)

	routerInfo := &core.RouterInfo{IP: "192.168.0.4", MAC: "02:00:00:00:00:04", UPnP: true, NATPMP: true, ExternalIP: "192.168.100.2", Reach: "outer_ring"}
	if err := env.store.SaveRouterState(ctx, string(testNet.Key), routerInfo, at); err != nil {
		t.Fatalf("SaveRouterState: %v", err)
	}
	pm := core.PortMapping{Protocol: "TCP", ExternalPort: 8080, InternalIP: "192.168.0.55", InternalPort: 80, Source: "upnp", DeviceID: &pc.ID}
	if err := env.store.ReconcilePortMappings(ctx, string(testNet.Key), []core.PortMapping{pm}, at); err != nil {
		t.Fatalf("ReconcilePortMappings: %v", err)
	}

	resp := env.doJSON(http.MethodGet, "/api/v1/security", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var report core.SecurityReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if report.Router == nil {
		t.Fatal("router = nil, want el RouterInfo guardado")
	}
	if report.Router.MAC != "02:00:00:00:00:04" || report.Router.Reach != "outer_ring" {
		t.Fatalf("router = %+v, want MAC/Reach del guardado", report.Router)
	}
	if len(report.Router.PortMappings) != 1 {
		t.Fatalf("portMappings = %+v, want 1", report.Router.PortMappings)
	}
	gotPM := report.Router.PortMappings[0]
	if gotPM.DeviceID == nil || *gotPM.DeviceID != pc.ID {
		t.Fatalf("portMapping.deviceId = %v, want %d", gotPM.DeviceID, pc.ID)
	}
	if gotPM.FirstSeen == nil || gotPM.LastSeen == nil {
		t.Fatal("portMapping.firstSeen/lastSeen = nil, want la hora de ReconcilePortMappings (enriquecido)")
	}

	var foundRisky bool
	for _, f := range report.Findings {
		if f.Kind == core.KindRiskyService {
			foundRisky = true
		}
	}
	if !foundRisky {
		t.Fatalf("findings = %+v, want un RISKY_SERVICE (445 abierto)", report.Findings)
	}
}

// El anillo exterior sale como OuterRing, con sus equipos en forma corta
// (SecurityDevice), sin mezclarse con los del interior.
func TestSecurity_AnilloExterior(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	env.seedDevice(testOuterNet, core.Device{IP: "192.168.100.3", OpenPorts: []int{80, 443}}, at)

	resp := env.doJSON(http.MethodGet, "/api/v1/security", token, nil)
	defer resp.Body.Close()
	var report core.SecurityReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.OuterRing == nil {
		t.Fatal("outerRing = nil, want la red exterior")
	}
	if report.OuterRing.Network != string(testOuterNet.Key) {
		t.Fatalf("outerRing.network = %q, want %q", report.OuterRing.Network, testOuterNet.Key)
	}
	if len(report.OuterRing.Devices) != 1 || report.OuterRing.Devices[0].Device.IP != "192.168.100.3" {
		t.Fatalf("outerRing.devices = %+v, want 1 equipo en 192.168.100.3", report.OuterRing.Devices)
	}
}

// Una alerta ROUTER_CHANGED de hace menos de 24h debe reflejarse como
// hallazgo del informe (ver core.BuildSecurityReport), que es exactamente
// lo que las alertas recientes existen para alimentar.
func TestSecurity_AlertaRecienteSeReflejaComoHallazgo(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	now := time.Now().UTC()

	env.seedAlert(alerts.Alert{Type: alerts.RouterChanged, Severity: alerts.High, At: now.Add(-1 * time.Hour),
		Title: "t", Body: "b", Network: string(testNet.Key), DedupKey: "rc-1"})

	resp := env.doJSON(http.MethodGet, "/api/v1/security", token, nil)
	defer resp.Body.Close()
	var report core.SecurityReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found bool
	for _, f := range report.Findings {
		if f.Kind == core.KindRouterChanged {
			found = true
		}
	}
	if !found {
		t.Fatalf("findings = %+v, want ROUTER_CHANGED (alerta de hace 1h)", report.Findings)
	}
}

// GET /api/v1/networks (v1.2) debe traer "role" para cada red.
func TestNetworks_IncluyeRole(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	env.seedDevice(testNet, core.Device{IP: "192.168.0.10"}, at)
	env.seedDevice(testOuterNet, core.Device{IP: "192.168.100.3"}, at)

	resp := env.doJSON(http.MethodGet, "/api/v1/networks", token, nil)
	defer resp.Body.Close()
	var nets []store.Network
	if err := json.NewDecoder(resp.Body).Decode(&nets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	roles := map[string]string{}
	for _, n := range nets {
		roles[n.Key] = n.Role
	}
	if roles[string(testNet.Key)] != "inner" {
		t.Fatalf("role de %q = %q, want inner", testNet.Key, roles[string(testNet.Key)])
	}
	if roles[string(testOuterNet.Key)] != "outer" {
		t.Fatalf("role de %q = %q, want outer", testOuterNet.Key, roles[string(testOuterNet.Key)])
	}
}

// GET/PUT /api/v1/alert-settings debe traer y aceptar las 4 claves nuevas.
func TestAlertSettings_ClavesDeFase5(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	resp := env.doJSON(http.MethodGet, "/api/v1/alert-settings", token, nil)
	defer resp.Body.Close()
	var raw map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"ROUTER_CHANGED", "ARP_CONFLICT", "PORT_MAPPING_ADDED", "RISKY_SERVICE"} {
		if !raw[key] {
			t.Fatalf("alert-settings[%s] = %v, want true por defecto (raw=%v)", key, raw[key], raw)
		}
	}

	raw["ARP_CONFLICT"] = false
	resp2 := env.doJSON(http.MethodPut, "/api/v1/alert-settings", token, raw)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", resp2.StatusCode)
	}

	resp3 := env.doJSON(http.MethodGet, "/api/v1/alert-settings", token, nil)
	defer resp3.Body.Close()
	var raw2 map[string]bool
	if err := json.NewDecoder(resp3.Body).Decode(&raw2); err != nil {
		t.Fatalf("decode 2: %v", err)
	}
	if raw2["ARP_CONFLICT"] {
		t.Fatal("ARP_CONFLICT sigue en true tras el PUT")
	}
	if !raw2["ROUTER_CHANGED"] {
		t.Fatal("ROUTER_CHANGED cambió a false sin que el PUT lo pidiera")
	}
}

// --- Hallazgos reconocidos (v1.3): PUT/DELETE /api/v1/security/acknowledged/{findingId} ---

func TestAcknowledged_SinTokenDa401(t *testing.T) {
	env := newTestEnv(t)
	findingID := url.PathEscape("RISKY_SERVICE|dev:1|445")

	resp := env.doJSON(http.MethodPut, "/api/v1/security/acknowledged/"+findingID, "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("PUT status = %d, want 401", resp.StatusCode)
	}
	resp2 := env.doJSON(http.MethodDelete, "/api/v1/security/acknowledged/"+findingID, "", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("DELETE status = %d, want 401", resp2.StatusCode)
	}
}

// Ciclo completo: PUT marca el hallazgo, GET /security lo refleja
// (acknowledged=true, sin restar del puntaje) y DELETE lo deshace.
func TestAcknowledged_CicloCompleto(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	pc := env.seedDevice(testNet, core.Device{IP: "192.168.0.55", MAC: "aa:bb:cc:dd:ee:01", OpenPorts: []int{445}}, at)
	// El id estable usa "dev:<id>" (ver core.findingWho / docs/agente-api.md v1.3).
	findingID := fmt.Sprintf("RISKY_SERVICE|dev:%d|445", pc.ID)
	encoded := url.PathEscape(findingID)

	// Antes de reconocer: el hallazgo resta del puntaje.
	before := env.doJSON(http.MethodGet, "/api/v1/security", token, nil)
	defer before.Body.Close()
	var reportBefore core.SecurityReport
	json.NewDecoder(before.Body).Decode(&reportBefore)
	if reportBefore.Score >= 100 {
		t.Fatalf("score antes = %d, want < 100 (RISKY_SERVICE sin reconocer)", reportBefore.Score)
	}

	putResp := env.doJSON(http.MethodPut, "/api/v1/security/acknowledged/"+encoded, token, map[string]string{"note": "lo uso Frigate"})
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", putResp.StatusCode)
	}

	after := env.doJSON(http.MethodGet, "/api/v1/security", token, nil)
	defer after.Body.Close()
	var reportAfter core.SecurityReport
	if err := json.NewDecoder(after.Body).Decode(&reportAfter); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// El equipo sembrado no tiene nombre ni identidad, así que además de
	// RISKY_SERVICE(445) sale UNKNOWN_DEVICES: ese SÍ sigue restando (no es
	// el hallazgo reconocido). Solo el reconocido no debe restar.
	if reportAfter.Score <= reportBefore.Score {
		t.Fatalf("score tras reconocer = %d, want más que antes (%d): el hallazgo reconocido ya no resta", reportAfter.Score, reportBefore.Score)
	}
	var found bool
	for _, f := range reportAfter.Findings {
		if f.ID == findingID {
			found = true
			if !f.Acknowledged {
				t.Errorf("Finding %q.Acknowledged = false, want true", f.ID)
			}
		}
	}
	if !found {
		t.Fatalf("no se encontró el hallazgo %q en %+v", findingID, reportAfter.Findings)
	}

	// DELETE deshace el reconocimiento.
	delResp := env.doJSON(http.MethodDelete, "/api/v1/security/acknowledged/"+encoded, token, nil)
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", delResp.StatusCode)
	}

	final := env.doJSON(http.MethodGet, "/api/v1/security", token, nil)
	defer final.Body.Close()
	var reportFinal core.SecurityReport
	json.NewDecoder(final.Body).Decode(&reportFinal)
	if reportFinal.Score != reportBefore.Score {
		t.Fatalf("score tras deshacer = %d, want %d (igual que antes de reconocer: vuelve a restar)", reportFinal.Score, reportBefore.Score)
	}
}

// PUT sin cuerpo también debe funcionar (el "note" es opcional, ver el contrato).
func TestAcknowledged_PutSinCuerpo(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	resp := env.doJSON(http.MethodPut, "/api/v1/security/acknowledged/"+url.PathEscape("PORT_MAPPING|dev:12|7070/TCP"), token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (cuerpo opcional)", resp.StatusCode)
	}
}

// DELETE de un hallazgo que nunca se reconoció no es un error.
func TestAcknowledged_DeleteDeAlgoQueNoExiste_204(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	resp := env.doJSON(http.MethodDelete, "/api/v1/security/acknowledged/"+url.PathEscape("RISKY_SERVICE|dev:1|445"), token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}
