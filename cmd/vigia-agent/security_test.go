package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/api"
	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/webhooks"
	"github.com/baxner/vigia/core"
)

// testAgentEnv junta lo que processInnerAlerts/processOuterAlerts/
// recordRouterInfo/recordOuterScan necesitan, sin tocar la red ni el reloj
// real: un store en un archivo temporal, un Hub y un Sender de verdad (no
// hace falta falsearlos: no salen a ninguna parte si no hay webhooks
// registrados) y un logger que no ensucia la salida de `go test`.
type testAgentEnv struct {
	t      *testing.T
	store  *store.Store
	hub    *api.Hub
	sender *webhooks.Sender
	id     agent.Identity
	log    *slog.Logger
}

func newTestAgentEnv(t *testing.T) *testAgentEnv {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return &testAgentEnv{
		t: t, store: st, hub: api.NewHub(), sender: webhooks.NewSender("test"),
		id:  agent.Identity{ID: "agent-test", Name: "Agente de prueba"},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

const testInnerNetKey = "192.168.0.4|192.168.0.0/24"

func (e *testAgentEnv) seedKnownDevice(ip, mac string, openPorts []int, at time.Time) history.KnownDevice {
	e.t.Helper()
	net := store.NetworkInfo{Key: testInnerNetKey, Subnet: "192.168.0.0/24", Gateway: "192.168.0.4", Role: "inner"}
	dev := core.Device{IP: ip, MAC: mac, OpenPorts: openPorts}
	if _, err := e.store.RecordScan(context.Background(), net, []core.Device{dev}, at); err != nil {
		e.t.Fatalf("RecordScan: %v", err)
	}
	known, err := e.store.DevicesByNetwork(context.Background(), testInnerNetKey)
	if err != nil {
		e.t.Fatalf("DevicesByNetwork: %v", err)
	}
	for _, d := range known {
		if d.LastIP == ip {
			return d
		}
	}
	e.t.Fatalf("no se encontró el equipo recién sembrado %s", ip)
	return history.KnownDevice{}
}

func (e *testAgentEnv) alertsOfType(typ alerts.Type) []store.Alert {
	e.t.Helper()
	all, err := e.store.Alerts(context.Background(), 0, 1000)
	if err != nil {
		e.t.Fatalf("Alerts: %v", err)
	}
	var out []store.Alert
	for _, a := range all {
		if a.Type == typ {
			out = append(out, a)
		}
	}
	return out
}

// --- ROUTER_CHANGED / ARP_CONFLICT / RISKY_SERVICE vía processInnerAlerts ---

func TestProcessInnerAlerts_RouterChanged(t *testing.T) {
	env := newTestAgentEnv(t)
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	processInnerAlerts(context.Background(), env.store, env.hub, env.sender, env.id, testInnerNetKey,
		nil, nil, "192.168.0.4", "02:00:00:00:00:04", "aa:bb:cc:dd:ee:ff", at, false, env.log)

	got := env.alertsOfType(alerts.RouterChanged)
	if len(got) != 1 {
		t.Fatalf("alertas ROUTER_CHANGED = %d, want 1", len(got))
	}
}

func TestProcessInnerAlerts_SinCambioDeMAC_NoAlerta(t *testing.T) {
	env := newTestAgentEnv(t)
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	processInnerAlerts(context.Background(), env.store, env.hub, env.sender, env.id, testInnerNetKey,
		nil, nil, "192.168.0.4", "02:00:00:00:00:04", "02:00:00:00:00:04", at, false, env.log)

	if got := env.alertsOfType(alerts.RouterChanged); len(got) != 0 {
		t.Fatalf("alertas ROUTER_CHANGED = %d, want 0 (misma MAC)", len(got))
	}
}

func TestProcessInnerAlerts_ARPConflict(t *testing.T) {
	env := newTestAgentEnv(t)
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	// Un equipo conocido con la MAC que, en ESTE escaneo, contestó en la IP del gateway.
	env.seedKnownDevice("192.168.0.55", "aa:bb:cc:dd:ee:01", nil, at)
	scanned := []core.Device{{IP: "192.168.0.4", MAC: "aa:bb:cc:dd:ee:01"}, {IP: "192.168.0.55", MAC: "aa:bb:cc:dd:ee:01"}}

	processInnerAlerts(context.Background(), env.store, env.hub, env.sender, env.id, testInnerNetKey,
		nil, scanned, "192.168.0.4", "", "", at, false, env.log)

	got := env.alertsOfType(alerts.ArpConflict)
	if len(got) != 1 {
		t.Fatalf("alertas ARP_CONFLICT = %d, want 1", len(got))
	}
}

func TestProcessInnerAlerts_RiskyService_LineaBaseLuegoAlerta(t *testing.T) {
	env := newTestAgentEnv(t)
	at0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	// El PC de la casa real (ver docs/plan.md): ya tenía estos
	// puertos abiertos ANTES de que esta regla existiera.
	env.seedKnownDevice("192.168.0.55", "aa:bb:cc:dd:ee:02", []int{139, 445, 554, 3389}, at0)

	processInnerAlerts(context.Background(), env.store, env.hub, env.sender, env.id, testInnerNetKey,
		nil, nil, "192.168.0.4", "", "", at0, false, env.log)
	if got := env.alertsOfType(alerts.RiskyService); len(got) != 0 {
		t.Fatalf("primera corrida: alertas RISKY_SERVICE = %d, want 0 (línea base)", len(got))
	}

	// Un segundo equipo abre un puerto de riesgo DESPUÉS de la línea base:
	// ese sí debe avisar.
	at1 := at0.Add(5 * time.Minute)
	env.seedKnownDevice("192.168.0.63", "aa:bb:cc:dd:ee:03", []int{23}, at1)
	processInnerAlerts(context.Background(), env.store, env.hub, env.sender, env.id, testInnerNetKey,
		nil, nil, "192.168.0.4", "", "", at1, false, env.log)

	got := env.alertsOfType(alerts.RiskyService)
	if len(got) != 1 {
		t.Fatalf("segunda corrida: alertas RISKY_SERVICE = %d, want 1 (equipo nuevo con Telnet)", len(got))
	}
	if got[0].Severity != alerts.High {
		t.Fatalf("severity = %s, want high (Telnet)", got[0].Severity)
	}

	// Una tercera corrida con el MISMO equipo/puerto no debe repetir.
	at2 := at1.Add(5 * time.Minute)
	processInnerAlerts(context.Background(), env.store, env.hub, env.sender, env.id, testInnerNetKey,
		nil, nil, "192.168.0.4", "", "", at2, false, env.log)
	if got := env.alertsOfType(alerts.RiskyService); len(got) != 1 {
		t.Fatalf("tercera corrida: alertas RISKY_SERVICE = %d, want seguir en 1 (no se repite)", len(got))
	}
}

// --- NEW_DEVICE: processInnerAlerts vs. processOuterAlerts ---

func TestProcessInnerAlerts_NewDevice_CuerpoInterior(t *testing.T) {
	env := newTestAgentEnv(t)
	at0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	// Línea base: un primer equipo.
	env.seedKnownDevice("192.168.0.10", "aa:bb:cc:dd:ee:04", nil, at0)

	// Un equipo nuevo, bien después de la línea base.
	at1 := at0.Add(time.Hour)
	ctx := context.Background()
	net := store.NetworkInfo{Key: testInnerNetKey, Subnet: "192.168.0.0/24", Gateway: "192.168.0.4", Role: "inner"}
	events, err := env.store.RecordScan(ctx, net, []core.Device{{IP: "192.168.0.10", MAC: "aa:bb:cc:dd:ee:04"}, {IP: "192.168.0.24", MAC: "aa:bb:cc:dd:ee:05"}}, at1)
	if err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey, events, nil, "192.168.0.4", "", "", at1, false, env.log)

	got := env.alertsOfType(alerts.NewDevice)
	if len(got) != 1 {
		t.Fatalf("alertas NEW_DEVICE = %d, want 1", len(got))
	}
	if got[0].Body == "" || got[0].Body == "192.168.0.24 se conectó por primera vez a la red de delante de tu router (la del módem)." {
		t.Fatalf("body = %q, want el texto del anillo INTERIOR (sin \"de delante de tu router\")", got[0].Body)
	}
}

func TestProcessOuterAlerts_NewDevice_CuerpoExterior(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	at0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	const outerKey = "192.168.100.1|192.168.100.0/24"
	net := store.NetworkInfo{Key: outerKey, Subnet: "192.168.100.0/24", Gateway: "192.168.100.1", Role: "outer"}

	// Línea base del anillo exterior.
	if _, err := env.store.RecordScan(ctx, net, []core.Device{{IP: "192.168.100.2"}}, at0); err != nil {
		t.Fatalf("RecordScan (base): %v", err)
	}

	at1 := at0.Add(time.Hour)
	events, err := env.store.RecordScan(ctx, net, []core.Device{{IP: "192.168.100.2"}, {IP: "192.168.100.5"}}, at1)
	if err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	processOuterAlerts(ctx, env.store, env.hub, env.sender, env.id, outerKey, events, env.log)

	got := env.alertsOfType(alerts.NewDevice)
	if len(got) != 1 {
		t.Fatalf("alertas NEW_DEVICE = %d, want 1", len(got))
	}
	want := "192.168.100.5 se conectó por primera vez a la red de delante de tu router (la del módem)."
	if got[0].Body != want {
		t.Fatalf("body = %q, want %q", got[0].Body, want)
	}
}

// recordOuterScan nunca debe tocar la salud del agente (ver
// docs/plan.md, Fase 5: "la salud se evalúa SOLO con el anillo
// interior"): ni un AGENT_BLIND/AGENT_RECOVERED, ni cambiar agent_health,
// sin importar cuántos (o pocos) equipos traiga el escaneo exterior.
func TestRecordOuterScan_NuncaEvaluaSalud(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	before, err := env.store.AgentHealthState(ctx)
	if err != nil {
		t.Fatalf("AgentHealthState: %v", err)
	}

	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	res := &core.ScanResult{Subnet: "192.168.100.0/24", Gateway: "192.168.100.1", Devices: nil, StartedAt: at}
	recordOuterScan(ctx, env.store, env.hub, env.sender, env.id, res, at, env.log)

	after, err := env.store.AgentHealthState(ctx)
	if err != nil {
		t.Fatalf("AgentHealthState 2: %v", err)
	}
	if before != after {
		t.Fatalf("agent_health cambió (%v -> %v) tras un escaneo exterior vacío, want que no se toque nunca", before, after)
	}
	if got := env.alertsOfType(alerts.AgentBlind); len(got) != 0 {
		t.Fatalf("alertas AGENT_BLIND = %d, want 0 (el anillo exterior no evalúa salud)", len(got))
	}
}

// --- recordRouterInfo: línea base de redirecciones sin tormenta ---

func TestRecordRouterInfo_LineaBaseLuegoAlertaPorRedireccionNueva(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	at0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	// Primera consulta al router: ya trae 2 redirecciones de antes de
	// actualizar. No debe avisar
	// nada: es la línea base.
	info := &core.RouterInfo{
		IP: "192.168.0.4", MAC: "02:00:00:00:00:04", UPnP: true, ExternalIP: "192.168.100.2", Reach: "outer_ring", MappingsComplete: true,
		PortMappings: []core.PortMapping{
			{Protocol: "UDP", ExternalPort: 41641, InternalIP: "192.168.0.55", InternalPort: 41641, Description: "Tailscale", Source: "upnp"},
			{Protocol: "TCP", ExternalPort: 7070, InternalIP: "192.168.0.63", InternalPort: 7070, Description: "AnyDesk", Source: "upnp"},
		},
	}
	recordRouterInfo(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey, info, at0, env.log)

	if got := env.alertsOfType(alerts.PortMappingAdded); len(got) != 0 {
		t.Fatalf("primera consulta: alertas PORT_MAPPING_ADDED = %d, want 0 (línea base)", len(got))
	}
	active, err := env.store.ActivePortMappings(ctx, testInnerNetKey)
	if err != nil {
		t.Fatalf("ActivePortMappings: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("redirecciones activas = %d, want 2 (igual se guardaron)", len(active))
	}
	baseline, err := env.store.PortMappingsBaselineDone(ctx)
	if err != nil {
		t.Fatalf("PortMappingsBaselineDone: %v", err)
	}
	if !baseline {
		t.Fatal("baseline = false, want true tras la primera consulta")
	}

	// Segunda consulta, 5 minutos después: aparece una redirección nueva
	// de verdad (HyperTorrent). Esta sí debe avisar, y solo una vez.
	at1 := at0.Add(5 * time.Minute)
	info2 := &core.RouterInfo{
		IP: "192.168.0.4", MAC: "02:00:00:00:00:04", UPnP: true, ExternalIP: "192.168.100.2", Reach: "outer_ring", MappingsComplete: true,
		PortMappings: append(append([]core.PortMapping{}, info.PortMappings...),
			core.PortMapping{Protocol: "TCP", ExternalPort: 51413, InternalIP: "192.168.0.105", InternalPort: 51413, Description: "HyperTorrent", Source: "upnp"}),
	}
	recordRouterInfo(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey, info2, at1, env.log)

	got := env.alertsOfType(alerts.PortMappingAdded)
	if len(got) != 1 {
		t.Fatalf("segunda consulta: alertas PORT_MAPPING_ADDED = %d, want 1", len(got))
	}
	active2, err := env.store.ActivePortMappings(ctx, testInnerNetKey)
	if err != nil {
		t.Fatalf("ActivePortMappings 2: %v", err)
	}
	if len(active2) != 3 {
		t.Fatalf("redirecciones activas = %d, want 3", len(active2))
	}
}

func TestRecordRouterInfo_LlenaDeviceIDDeEquipoConocido(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	pc := env.seedKnownDevice("192.168.0.55", "aa:bb:cc:dd:ee:06", nil, at)

	info := &core.RouterInfo{IP: "192.168.0.4", MappingsComplete: true, PortMappings: []core.PortMapping{
		{Protocol: "TCP", ExternalPort: 8080, InternalIP: "192.168.0.55", InternalPort: 80, Source: "upnp"},
	}}
	recordRouterInfo(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey, info, at, env.log)

	active, err := env.store.ActivePortMappings(ctx, testInnerNetKey)
	if err != nil || len(active) != 1 {
		t.Fatalf("ActivePortMappings: active=%+v err=%v", active, err)
	}
	if active[0].DeviceID == nil || *active[0].DeviceID != pc.ID {
		t.Fatalf("deviceId = %v, want %d (el equipo de 192.168.0.55)", active[0].DeviceID, pc.ID)
	}
}

// --- IMPORTANT_OFFLINE/IMPORTANT_ONLINE (Fase 5.1): reloj fijo ---

// markImportant marca dev como importante vía PATCH-equivalente directo al
// store (no hace falta pasar por la API HTTP para esto).
func (e *testAgentEnv) markImportant(deviceID int64) {
	e.t.Helper()
	patch := store.CustomizationPatch{SetImportant: true, Important: true}
	if _, err := e.store.UpdateCustomization(context.Background(), deviceID, patch); err != nil {
		e.t.Fatalf("UpdateCustomization: %v", err)
	}
}

// Caso real: el Wi-Fi del teléfono se duerme de
// madrugada y vuelve a los pocos minutos. Con reloj fijo: si falta y
// vuelve a los 5 min (menos del umbral de 15), ningún aviso — ni de
// desconexión ni de reconexión.
func TestImportantOffline_VuelveA5Min_NingunAviso(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	dev := env.seedKnownDevice("192.168.0.24", "aa:bb:cc:dd:ee:10", nil, t0)
	env.markImportant(dev.ID)

	net := store.NetworkInfo{Key: testInnerNetKey, Subnet: "192.168.0.0/24", Gateway: "192.168.0.4", Role: "inner"}

	// Escaneo a los 5 min: el equipo no aparece (el Wi-Fi se durmió).
	t1 := t0.Add(5 * time.Minute)
	events1, err := env.store.RecordScan(ctx, net, nil, t1)
	if err != nil {
		t.Fatalf("RecordScan (falta): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events1, nil, "192.168.0.4", "", "", t1, false, env.log)
	if got := env.alertsOfType(alerts.ImportantOffline); len(got) != 0 {
		t.Fatalf("alertas IMPORTANT_OFFLINE = %d, want 0 (5 min < 15 min de umbral)", len(got))
	}

	// Vuelve un minuto después (6 min desde que se vio por última vez).
	t2 := t1.Add(time.Minute)
	events2, err := env.store.RecordScan(ctx, net, []core.Device{{IP: "192.168.0.24", MAC: "aa:bb:cc:dd:ee:10"}}, t2)
	if err != nil {
		t.Fatalf("RecordScan (vuelve): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events2, nil, "192.168.0.4", "", "", t2, false, env.log)

	if got := env.alertsOfType(alerts.ImportantOffline); len(got) != 0 {
		t.Fatalf("alertas IMPORTANT_OFFLINE = %d, want 0 (nunca cruzó el umbral)", len(got))
	}
	if got := env.alertsOfType(alerts.ImportantOnline); len(got) != 0 {
		t.Fatalf("alertas IMPORTANT_ONLINE = %d, want 0 (nunca se avisó la caída, no hay regreso que avisar)", len(got))
	}
}

// Con reloj fijo: el equipo se va y NO vuelve dentro de un escaneo a los
// 20 minutos (sigue ausente) -> un aviso de desconexión. Al volver,
// un aviso de reconexión. Ninguno de los dos se repite.
func TestImportantOffline_Tarda20Min_UnAvisoDeCadaUno(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	dev := env.seedKnownDevice("192.168.0.24", "aa:bb:cc:dd:ee:11", nil, t0)
	env.markImportant(dev.ID)

	net := store.NetworkInfo{Key: testInnerNetKey, Subnet: "192.168.0.0/24", Gateway: "192.168.0.4", Role: "inner"}

	// Escaneo a los 20 min: el equipo no aparece (falta). La gracia del
	// historial (10 min) ya comprometió WENT_OFFLINE antes de esto, pero
	// eso ya no genera la alerta (ver Fase 5.1): processInnerAlerts debe
	// avisar IMPORTANT_OFFLINE mirando el estado actual (known).
	t1 := t0.Add(20 * time.Minute)
	events1, err := env.store.RecordScan(ctx, net, nil, t1)
	if err != nil {
		t.Fatalf("RecordScan (falta): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events1, nil, "192.168.0.4", "", "", t1, false, env.log)

	got := env.alertsOfType(alerts.ImportantOffline)
	if len(got) != 1 {
		t.Fatalf("alertas IMPORTANT_OFFLINE = %d, want 1 (20 min > 15 min de umbral)", len(got))
	}

	// Otro escaneo 5 min después (25 min desde t0), SIGUE sin aparecer: no
	// debe repetir el aviso (mismo episodio, misma DedupKey).
	t2 := t1.Add(5 * time.Minute)
	events2, err := env.store.RecordScan(ctx, net, nil, t2)
	if err != nil {
		t.Fatalf("RecordScan (sigue faltando): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events2, nil, "192.168.0.4", "", "", t2, false, env.log)
	if got := env.alertsOfType(alerts.ImportantOffline); len(got) != 1 {
		t.Fatalf("alertas IMPORTANT_OFFLINE tras otro escaneo sin aparecer = %d, want seguir en 1", len(got))
	}

	// Vuelve: un aviso de reconexión, uno solo.
	t3 := t2.Add(5 * time.Minute)
	events3, err := env.store.RecordScan(ctx, net, []core.Device{{IP: "192.168.0.24", MAC: "aa:bb:cc:dd:ee:11"}}, t3)
	if err != nil {
		t.Fatalf("RecordScan (vuelve): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events3, nil, "192.168.0.4", "", "", t3, false, env.log)

	gotOnline := env.alertsOfType(alerts.ImportantOnline)
	if len(gotOnline) != 1 {
		t.Fatalf("alertas IMPORTANT_ONLINE = %d, want 1", len(gotOnline))
	}

	// Un escaneo más, ya en línea desde antes: no debe repetir IMPORTANT_ONLINE.
	t4 := t3.Add(5 * time.Minute)
	events4, err := env.store.RecordScan(ctx, net, []core.Device{{IP: "192.168.0.24", MAC: "aa:bb:cc:dd:ee:11"}}, t4)
	if err != nil {
		t.Fatalf("RecordScan (sigue en línea): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events4, nil, "192.168.0.4", "", "", t4, false, env.log)
	if got := env.alertsOfType(alerts.ImportantOnline); len(got) != 1 {
		t.Fatalf("alertas IMPORTANT_ONLINE tras seguir en línea = %d, want seguir en 1 (no se repite)", len(got))
	}
}

// Si el equipo vuelve ANTES de los 15 min (nunca se avisó la caída), no
// debe avisar IMPORTANT_ONLINE tampoco.
func TestImportantOnline_SinOfflinePrevio_NoAvisa(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	dev := env.seedKnownDevice("192.168.0.24", "aa:bb:cc:dd:ee:12", nil, t0)
	env.markImportant(dev.ID)

	net := store.NetworkInfo{Key: testInnerNetKey, Subnet: "192.168.0.0/24", Gateway: "192.168.0.4", Role: "inner"}

	// Falta 5 min (menos del umbral): processInnerAlerts no debe marcar pendiente.
	t1 := t0.Add(5 * time.Minute)
	events1, _ := env.store.RecordScan(ctx, net, nil, t1)
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events1, nil, "192.168.0.4", "", "", t1, false, env.log)

	// Vuelve: sin pendiente, no debe avisar IMPORTANT_ONLINE.
	t2 := t1.Add(time.Minute)
	events2, _ := env.store.RecordScan(ctx, net, []core.Device{{IP: "192.168.0.24", MAC: "aa:bb:cc:dd:ee:12"}}, t2)
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events2, nil, "192.168.0.4", "", "", t2, false, env.log)

	if got := env.alertsOfType(alerts.ImportantOnline); len(got) != 0 {
		t.Fatalf("alertas IMPORTANT_ONLINE = %d, want 0 (nunca se avisó la caída: 5 min < 15 min)", len(got))
	}
	if got := env.alertsOfType(alerts.ImportantOffline); len(got) != 0 {
		t.Fatalf("alertas IMPORTANT_OFFLINE = %d, want 0", len(got))
	}
}

// --- justWoke: suspensión de la máquina (ver docs/agente-api.md,
// "Suspensión") ---

// Lo que pasaría SIN la excepción de justWoke: la máquina se duerme 2
// horas; al despertar, el primer escaneo ve a un equipo importante
// ausente desde antes de dormir — "lleva 2h sin conexión" dispararía
// IMPORTANT_OFFLINE de inmediato, aunque el equipo nunca se fue de
// verdad (solo el agente no pudo preguntar mientras dormía).
// justWoke=true debe saltarse ESE escaneo.
func TestImportantOffline_JustWoke_NoAvisaEseEscaneo(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	dev := env.seedKnownDevice("192.168.0.24", "aa:bb:cc:dd:ee:20", nil, t0)
	env.markImportant(dev.ID)

	net := store.NetworkInfo{Key: testInnerNetKey, Subnet: "192.168.0.0/24", Gateway: "192.168.0.4", Role: "inner"}

	// La máquina duerme 2 horas; al despertar, el equipo no aparece en
	// ESTE primer escaneo (podría ser real o solo que la red tarda unos
	// segundos en reasociarse) — justWoke=true debe saltarse el aviso.
	t1 := t0.Add(2 * time.Hour)
	events1, err := env.store.RecordScan(ctx, net, nil, t1)
	if err != nil {
		t.Fatalf("RecordScan (justo al despertar): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events1, nil, "192.168.0.4", "", "", t1, true, env.log)
	if got := env.alertsOfType(alerts.ImportantOffline); len(got) != 0 {
		t.Fatalf("alertas IMPORTANT_OFFLINE = %d, want 0 (justWoke debe saltarse este escaneo)", len(got))
	}

	// El SIGUIENTE escaneo, ya sin el hueco (justWoke=false): si de
	// verdad sigue faltando, ahí sí avisa — no se pierde, solo se
	// retrasa un ciclo.
	t2 := t1.Add(5 * time.Minute)
	events2, err := env.store.RecordScan(ctx, net, nil, t2)
	if err != nil {
		t.Fatalf("RecordScan (sigue faltando): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events2, nil, "192.168.0.4", "", "", t2, false, env.log)
	if got := env.alertsOfType(alerts.ImportantOffline); len(got) != 1 {
		t.Fatalf("alertas IMPORTANT_OFFLINE = %d, want 1 (el ciclo siguiente, ya sin justWoke, sí avisa)", len(got))
	}
}

// Si el equipo SÍ aparece en el primer escaneo tras despertar,
// IMPORTANT_ONLINE no se salta por justWoke: solo EvaluateImportantOffline
// se salta (ver el comentario de evaluateImportantConnectivity).
func TestImportantOnline_JustWoke_SigueAvisando(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	dev := env.seedKnownDevice("192.168.0.24", "aa:bb:cc:dd:ee:21", nil, t0)
	env.markImportant(dev.ID)

	net := store.NetworkInfo{Key: testInnerNetKey, Subnet: "192.168.0.0/24", Gateway: "192.168.0.4", Role: "inner"}

	// Un episodio de verdad, YA avisado (sin justWoke): el equipo lleva
	// 20 min sin conexión.
	t1 := t0.Add(20 * time.Minute)
	events1, _ := env.store.RecordScan(ctx, net, nil, t1)
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events1, nil, "192.168.0.4", "", "", t1, false, env.log)
	if got := env.alertsOfType(alerts.ImportantOffline); len(got) != 1 {
		t.Fatalf("alertas IMPORTANT_OFFLINE = %d, want 1 (episodio ya avisado antes de dormir)", len(got))
	}

	// La máquina duerme 2 horas; al despertar, el equipo SÍ aparece: el
	// aviso de reconexión no debe saltarse aunque justWoke sea true.
	t2 := t1.Add(2 * time.Hour)
	events2, err := env.store.RecordScan(ctx, net, []core.Device{{IP: "192.168.0.24", MAC: "aa:bb:cc:dd:ee:21"}}, t2)
	if err != nil {
		t.Fatalf("RecordScan (vuelve justo al despertar): %v", err)
	}
	processInnerAlerts(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey,
		events2, nil, "192.168.0.4", "", "", t2, true, env.log)
	if got := env.alertsOfType(alerts.ImportantOnline); len(got) != 1 {
		t.Fatalf("alertas IMPORTANT_ONLINE = %d, want 1 (IMPORTANT_ONLINE no se salta por justWoke)", len(got))
	}
}

// Caso real: una consulta que
// trae la lista a medias desactivaba las demás, y la siguiente las
// "redescubría". Con la lista incompleta no se toca nada ni se avisa.
func TestRecordRouterInfo_ListaIncompleta_NoDesactivaNiAvisa(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	at0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	tailscale := core.PortMapping{Protocol: "UDP", ExternalPort: 41641, InternalIP: "192.168.0.48", InternalPort: 41641, Source: "upnp"}
	anydesk := core.PortMapping{Protocol: "TCP", ExternalPort: 43957, InternalIP: "192.168.0.63", InternalPort: 7070, Description: "AnyDesk", Source: "upnp"}
	full := &core.RouterInfo{IP: "192.168.0.4", UPnP: true, MappingsComplete: true, PortMappings: []core.PortMapping{tailscale, anydesk}}
	recordRouterInfo(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey, full, at0, env.log) // línea base

	// Consulta cortada: solo alcanzó a leer una.
	partial := &core.RouterInfo{IP: "192.168.0.4", UPnP: true, MappingsComplete: false, PortMappings: []core.PortMapping{tailscale}}
	recordRouterInfo(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey, partial, at0.Add(2*time.Minute), env.log)
	active, err := env.store.ActivePortMappings(ctx, testInnerNetKey)
	if err != nil || len(active) != 2 {
		t.Fatalf("activas tras la lista incompleta = %d (err %v), want 2: no se debe desactivar nada", len(active), err)
	}

	// La siguiente, completa, con AnyDesk en otro puerto exterior: tampoco avisa.
	anydeskHoy := anydesk
	anydeskHoy.ExternalPort = 44559
	full2 := &core.RouterInfo{IP: "192.168.0.4", UPnP: true, MappingsComplete: true, PortMappings: []core.PortMapping{tailscale, anydeskHoy}}
	recordRouterInfo(ctx, env.store, env.hub, env.sender, env.id, testInnerNetKey, full2, at0.Add(4*time.Minute), env.log)
	if got := env.alertsOfType(alerts.PortMappingAdded); len(got) != 0 {
		t.Fatalf("alertas PORT_MAPPING_ADDED = %d, want 0 (nada nuevo de verdad)", len(got))
	}
}
