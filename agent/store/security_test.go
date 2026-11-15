package store

import (
	"context"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/core"
)

var securityT0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

// --- networks.role / NetworkByKey ---

func TestRecordScan_RoleVacioCaeAInner(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.10", "aa:bb:cc:dd:ee:01")}, securityT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	n, err := s.NetworkByKey(ctx, string(testNet.Key))
	if err != nil {
		t.Fatalf("NetworkByKey: %v", err)
	}
	if n.Role != "inner" {
		t.Fatalf("role = %q, want %q (testNet no fija Role)", n.Role, "inner")
	}
}

func TestRecordScan_RoleOuterSePersiste(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	outerNet := NetworkInfo{Key: "192.168.100.1|192.168.100.0/24", Subnet: "192.168.100.0/24", Gateway: "192.168.100.1", Role: "outer"}
	if _, err := s.RecordScan(ctx, outerNet, []core.Device{scanDev("192.168.100.3", "")}, securityT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	n, err := s.NetworkByKey(ctx, string(outerNet.Key))
	if err != nil {
		t.Fatalf("NetworkByKey: %v", err)
	}
	if n.Role != "outer" {
		t.Fatalf("role = %q, want outer", n.Role)
	}

	nets, err := s.Networks(ctx)
	if err != nil {
		t.Fatalf("Networks: %v", err)
	}
	if len(nets) != 1 || nets[0].Role != "outer" {
		t.Fatalf("Networks() = %+v, want 1 red con role=outer", nets)
	}
}

func TestNetworkByKey_NoExiste(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.NetworkByKey(context.Background(), "no-existe|192.168.9.0/24"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// --- router_state / ActivePortMappings ---

func TestRouterState_NilSiNuncaSeGuardo(t *testing.T) {
	s := openTestStore(t)
	info, err := s.RouterState(context.Background(), string(testNet.Key))
	if err != nil {
		t.Fatalf("RouterState: %v", err)
	}
	if info != nil {
		t.Fatalf("info = %+v, want nil (nunca se guardó)", info)
	}
}

func TestRouterState_GuardarYReleer(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	netKey := string(testNet.Key)

	in := &core.RouterInfo{IP: "192.168.0.4", MAC: "02:00:00:00:00:04", UPnP: true, NATPMP: true, ExternalIP: "192.168.100.2", Reach: "outer_ring"}
	if err := s.SaveRouterState(ctx, netKey, in, securityT0); err != nil {
		t.Fatalf("SaveRouterState: %v", err)
	}
	got, err := s.RouterState(ctx, netKey)
	if err != nil {
		t.Fatalf("RouterState: %v", err)
	}
	if got == nil {
		t.Fatal("got = nil, want el RouterInfo guardado")
	}
	if got.IP != in.IP || got.MAC != in.MAC || got.UPnP != in.UPnP || got.NATPMP != in.NATPMP ||
		got.ExternalIP != in.ExternalIP || got.Reach != in.Reach {
		t.Fatalf("got = %+v, want %+v", got, in)
	}
	if len(got.PortMappings) != 0 {
		t.Fatalf("PortMappings = %+v, want vacío (nada reconciliado aún)", got.PortMappings)
	}

	// Un segundo guardado reemplaza, no acumula (una fila por red).
	in2 := &core.RouterInfo{IP: "192.168.0.4", MAC: "aa:bb:cc:dd:ee:ff", UPnP: false, NATPMP: true, ExternalIP: "8.8.8.8", Reach: "internet"}
	if err := s.SaveRouterState(ctx, netKey, in2, securityT0.Add(time.Minute)); err != nil {
		t.Fatalf("SaveRouterState 2: %v", err)
	}
	got2, err := s.RouterState(ctx, netKey)
	if err != nil {
		t.Fatalf("RouterState 2: %v", err)
	}
	if got2.MAC != "aa:bb:cc:dd:ee:ff" || got2.Reach != "internet" {
		t.Fatalf("got2 = %+v, want el reemplazo", got2)
	}
}

func TestActivePortMappings_VacioSinReconciliar(t *testing.T) {
	s := openTestStore(t)
	got, err := s.ActivePortMappings(context.Background(), string(testNet.Key))
	if err != nil {
		t.Fatalf("ActivePortMappings: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want vacío", got)
	}
}

func mkPM(proto string, extPort int, ip string, intPort int) core.PortMapping {
	return core.PortMapping{Protocol: proto, ExternalPort: extPort, InternalIP: ip, InternalPort: intPort, Description: "cam", Source: "upnp"}
}

func TestReconcilePortMappings_InsertaActualizaReactivaYDesactiva(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	netKey := string(testNet.Key)

	// Ronda 1: dos redirecciones nuevas.
	m1 := mkPM("TCP", 8080, "192.168.0.55", 80)
	m2 := mkPM("TCP", 7000, "192.168.0.63", 7070)
	if err := s.ReconcilePortMappings(ctx, netKey, []core.PortMapping{m1, m2}, securityT0); err != nil {
		t.Fatalf("ReconcilePortMappings 1: %v", err)
	}
	active, err := s.ActivePortMappings(ctx, netKey)
	if err != nil {
		t.Fatalf("ActivePortMappings 1: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("active = %+v, want 2", active)
	}
	for _, pm := range active {
		if pm.FirstSeen == nil || !pm.FirstSeen.Equal(securityT0) {
			t.Fatalf("FirstSeen = %v, want %v", pm.FirstSeen, securityT0)
		}
	}

	// Ronda 2 (10 min después): m1 sigue, m2 desaparece, aparece m3 nueva.
	t1 := securityT0.Add(10 * time.Minute)
	m3 := mkPM("UDP", 51820, "192.168.0.105", 51820)
	if err := s.ReconcilePortMappings(ctx, netKey, []core.PortMapping{m1, m3}, t1); err != nil {
		t.Fatalf("ReconcilePortMappings 2: %v", err)
	}
	active2, err := s.ActivePortMappings(ctx, netKey)
	if err != nil {
		t.Fatalf("ActivePortMappings 2: %v", err)
	}
	if len(active2) != 2 {
		t.Fatalf("active2 = %+v, want 2 (m1 y m3; m2 se desactivó)", active2)
	}
	var gotM1 *core.PortMapping
	for i := range active2 {
		if active2[i].ExternalPort == 8080 {
			gotM1 = &active2[i]
		}
	}
	if gotM1 == nil {
		t.Fatal("m1 no está entre las activas")
	}
	if !gotM1.FirstSeen.Equal(securityT0) {
		t.Fatalf("m1.FirstSeen cambió a %v, want seguir siendo %v", gotM1.FirstSeen, securityT0)
	}
	if !gotM1.LastSeen.Equal(t1) {
		t.Fatalf("m1.LastSeen = %v, want %v (se refrescó)", gotM1.LastSeen, t1)
	}

	// Ronda 3: m2 reaparece. Debe reactivarse manteniendo su first_seen
	// original (ronda 1), no el de esta reaparición.
	t2 := t1.Add(10 * time.Minute)
	if err := s.ReconcilePortMappings(ctx, netKey, []core.PortMapping{m1, m2}, t2); err != nil {
		t.Fatalf("ReconcilePortMappings 3: %v", err)
	}
	active3, err := s.ActivePortMappings(ctx, netKey)
	if err != nil {
		t.Fatalf("ActivePortMappings 3: %v", err)
	}
	if len(active3) != 2 {
		t.Fatalf("active3 = %+v, want 2 (m1 y m2 reactivada; m3 se desactivó)", active3)
	}
	var gotM2 *core.PortMapping
	for i := range active3 {
		if active3[i].ExternalPort == 7000 {
			gotM2 = &active3[i]
		}
	}
	if gotM2 == nil {
		t.Fatal("m2 no se reactivó")
	}
	if !gotM2.FirstSeen.Equal(securityT0) {
		t.Fatalf("m2.FirstSeen = %v, want %v (el de la ronda 1, no la reaparición)", gotM2.FirstSeen, securityT0)
	}
}

func TestReconcilePortMappings_DeviceIDYDescripcionSePersisten(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	netKey := string(testNet.Key)

	// device_id es FOREIGN KEY (ver migration3): tiene que ser un equipo
	// que de verdad exista, igual que alerts.device_id (ver InsertAlert).
	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.55", "aa:bb:cc:dd:ee:09")}, securityT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil || len(known) != 1 {
		t.Fatalf("DevicesByNetwork: known=%v err=%v", known, err)
	}
	id := known[0].ID
	m := core.PortMapping{Protocol: "TCP", ExternalPort: 7070, InternalIP: "192.168.0.55", InternalPort: 7070, Description: "AnyDesk", Source: "upnp", DeviceID: &id}
	if err := s.ReconcilePortMappings(ctx, netKey, []core.PortMapping{m}, securityT0); err != nil {
		t.Fatalf("ReconcilePortMappings: %v", err)
	}
	active, err := s.ActivePortMappings(ctx, netKey)
	if err != nil {
		t.Fatalf("ActivePortMappings: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("active = %+v, want 1", active)
	}
	got := active[0]
	if got.DeviceID == nil || *got.DeviceID != id {
		t.Fatalf("DeviceID = %v, want %d", got.DeviceID, id)
	}
	if got.Description != "AnyDesk" {
		t.Fatalf("Description = %q, want AnyDesk", got.Description)
	}
}

// --- security_baseline ---

func TestPortMappingsBaseline_PorDefectoFalseYSePuedeMarcar(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	done, err := s.PortMappingsBaselineDone(ctx)
	if err != nil {
		t.Fatalf("PortMappingsBaselineDone: %v", err)
	}
	if done {
		t.Fatal("done = true, want false (base recién migrada)")
	}
	if err := s.SetPortMappingsBaselineDone(ctx); err != nil {
		t.Fatalf("SetPortMappingsBaselineDone: %v", err)
	}
	done2, err := s.PortMappingsBaselineDone(ctx)
	if err != nil {
		t.Fatalf("PortMappingsBaselineDone 2: %v", err)
	}
	if !done2 {
		t.Fatal("done2 = false, want true")
	}
}

func TestRiskyServicesBaseline_PorDefectoFalseYSePuedeMarcar(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if done, err := s.RiskyServicesBaselineDone(ctx); err != nil || done {
		t.Fatalf("done=%v err=%v, want false,nil", done, err)
	}
	if err := s.SetRiskyServicesBaselineDone(ctx); err != nil {
		t.Fatalf("SetRiskyServicesBaselineDone: %v", err)
	}
	if done, err := s.RiskyServicesBaselineDone(ctx); err != nil || !done {
		t.Fatalf("done=%v err=%v, want true,nil", done, err)
	}
}

// --- risky_service_seen ---

func TestSeenRiskyPorts_VacioYSeRegistra(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.55", "aa:bb:cc:dd:ee:02")}, securityT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil || len(known) != 1 {
		t.Fatalf("DevicesByNetwork: known=%v err=%v", known, err)
	}
	deviceID := known[0].ID

	seen, err := s.SeenRiskyPorts(ctx, string(testNet.Key))
	if err != nil {
		t.Fatalf("SeenRiskyPorts: %v", err)
	}
	if len(seen) != 0 {
		t.Fatalf("seen = %+v, want vacío", seen)
	}

	if err := s.RecordRiskyPortSeen(ctx, deviceID, 445, securityT0); err != nil {
		t.Fatalf("RecordRiskyPortSeen: %v", err)
	}
	seen2, err := s.SeenRiskyPorts(ctx, string(testNet.Key))
	if err != nil {
		t.Fatalf("SeenRiskyPorts 2: %v", err)
	}
	if !seen2[deviceID][445] {
		t.Fatalf("seen2 = %+v, want [%d][445]=true", seen2, deviceID)
	}

	// Repetir el mismo par no debe fallar (ON CONFLICT DO NOTHING).
	if err := s.RecordRiskyPortSeen(ctx, deviceID, 445, securityT0.Add(time.Hour)); err != nil {
		t.Fatalf("RecordRiskyPortSeen (repetido): %v", err)
	}
}

func TestSeenRiskyPorts_AisladoPorRed(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	otherNet := NetworkInfo{Key: "192.168.1.1|192.168.1.0/24", Subnet: "192.168.1.0/24", Gateway: "192.168.1.1"}

	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.55", "aa:bb:cc:dd:ee:03")}, securityT0); err != nil {
		t.Fatalf("RecordScan (red A): %v", err)
	}
	if _, err := s.RecordScan(ctx, otherNet, []core.Device{scanDev("192.168.1.55", "aa:bb:cc:dd:ee:04")}, securityT0); err != nil {
		t.Fatalf("RecordScan (red B): %v", err)
	}
	devA, _ := s.DevicesByNetwork(ctx, testNet.Key)

	if err := s.RecordRiskyPortSeen(ctx, devA[0].ID, 445, securityT0); err != nil {
		t.Fatalf("RecordRiskyPortSeen A: %v", err)
	}
	seenA, err := s.SeenRiskyPorts(ctx, string(testNet.Key))
	if err != nil {
		t.Fatalf("SeenRiskyPorts A: %v", err)
	}
	seenB, err := s.SeenRiskyPorts(ctx, string(otherNet.Key))
	if err != nil {
		t.Fatalf("SeenRiskyPorts B: %v", err)
	}
	if !seenA[devA[0].ID][445] {
		t.Fatalf("seenA = %+v, want el puerto de A", seenA)
	}
	if len(seenB) != 0 {
		t.Fatalf("seenB = %+v, want vacío (el visto fue en la red A, no B)", seenB)
	}
}

// --- alert_settings: las 4 claves nuevas ---

func TestAlertSettings_ClavesNuevas_DefaultTrueYRoundtrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	got, err := s.AlertSettings(ctx)
	if err != nil {
		t.Fatalf("AlertSettings: %v", err)
	}
	if !got.RouterChanged || !got.ArpConflict || !got.PortMappingAdded || !got.RiskyService {
		t.Fatalf("got = %+v, want las 4 claves nuevas en true por defecto", got)
	}

	got.ArpConflict = false
	got.RiskyService = false
	if err := s.SetAlertSettings(ctx, got); err != nil {
		t.Fatalf("SetAlertSettings: %v", err)
	}
	got2, err := s.AlertSettings(ctx)
	if err != nil {
		t.Fatalf("AlertSettings 2: %v", err)
	}
	if got2.ArpConflict || got2.RiskyService {
		t.Fatalf("got2 = %+v, want ArpConflict/RiskyService en false", got2)
	}
	if !got2.RouterChanged || !got2.PortMappingAdded {
		t.Fatalf("got2 = %+v, want RouterChanged/PortMappingAdded siguiendo en true", got2)
	}
}

// --- RecentAlerts ---

func TestRecentAlerts_FiltraPorVentana(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	old := alerts.Alert{Type: alerts.AgentBlind, Severity: alerts.Medium, At: securityT0.Add(-48 * time.Hour),
		Title: "t", Body: "b", Network: "net1", DedupKey: "k-vieja"}
	recent := alerts.Alert{Type: alerts.RouterChanged, Severity: alerts.High, At: securityT0.Add(-1 * time.Hour),
		Title: "t", Body: "b", Network: "net1", DedupKey: "k-reciente"}
	if _, _, err := s.InsertAlert(ctx, old); err != nil {
		t.Fatalf("InsertAlert (vieja): %v", err)
	}
	if _, _, err := s.InsertAlert(ctx, recent); err != nil {
		t.Fatalf("InsertAlert (reciente): %v", err)
	}

	got, err := s.RecentAlerts(ctx, securityT0.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("RecentAlerts: %v", err)
	}
	if len(got) != 1 || got[0].Type != alerts.RouterChanged {
		t.Fatalf("got = %+v, want solo la alerta reciente (ROUTER_CHANGED)", got)
	}
}

// --- important_offline_pending (Fase 5.1) ---

func TestImportantOfflinePending_SetGetClear(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	empty, err := s.ImportantOfflinePending(ctx)
	if err != nil || len(empty) != 0 {
		t.Fatalf("ImportantOfflinePending recién migrado = %+v err=%v, want vacío", empty, err)
	}

	// device_id es FK a devices(id): hace falta un equipo real.
	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.24", "aa:bb:cc:dd:ee:07")}, securityT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil || len(known) != 1 {
		t.Fatalf("DevicesByNetwork: known=%+v err=%v", known, err)
	}
	deviceID := known[0].ID

	if err := s.SetImportantOfflinePending(ctx, deviceID); err != nil {
		t.Fatalf("SetImportantOfflinePending: %v", err)
	}
	// Marcarlo de nuevo (mismo episodio, siguiente escaneo) no debe fallar.
	if err := s.SetImportantOfflinePending(ctx, deviceID); err != nil {
		t.Fatalf("SetImportantOfflinePending (otra vez): %v", err)
	}

	got, err := s.ImportantOfflinePending(ctx)
	if err != nil || !got[deviceID] {
		t.Fatalf("ImportantOfflinePending = %+v err=%v, want {%d: true}", got, err, deviceID)
	}

	if err := s.ClearImportantOfflinePending(ctx, deviceID); err != nil {
		t.Fatalf("ClearImportantOfflinePending: %v", err)
	}
	got2, err := s.ImportantOfflinePending(ctx)
	if err != nil || got2[deviceID] {
		t.Fatalf("ImportantOfflinePending tras limpiar = %+v err=%v, want sin el equipo", got2, err)
	}
	// Limpiar algo que no estaba marcado no debe fallar (idempotente).
	if err := s.ClearImportantOfflinePending(ctx, 999); err != nil {
		t.Fatalf("ClearImportantOfflinePending (inexistente): %v", err)
	}
}

// --- acknowledged_findings (v1.3) ---

func TestAcknowledged_SetGetRemove(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	empty, err := s.Acknowledgements(ctx)
	if err != nil || len(empty) != 0 {
		t.Fatalf("Acknowledgements recién migrado = %+v err=%v, want vacío", empty, err)
	}

	note := "el RTSP de la cámara, lo usa Frigate"
	at := securityT0
	if err := s.SetAcknowledged(ctx, "RISKY_SERVICE|dev:7|554", &note, at); err != nil {
		t.Fatalf("SetAcknowledged: %v", err)
	}
	got, err := s.Acknowledgements(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("Acknowledgements = %+v err=%v, want 1", got, err)
	}
	if got[0].FindingID != "RISKY_SERVICE|dev:7|554" || got[0].Note == nil || *got[0].Note != note || !got[0].At.Equal(at) {
		t.Fatalf("Acknowledgements[0] = %+v, want findingId/note/at de arriba", got[0])
	}

	// PUT de nuevo (cambia solo la nota): upsert, no un segundo registro.
	note2 := "actualizado"
	at2 := at.Add(time.Hour)
	if err := s.SetAcknowledged(ctx, "RISKY_SERVICE|dev:7|554", &note2, at2); err != nil {
		t.Fatalf("SetAcknowledged (upsert): %v", err)
	}
	got2, err := s.Acknowledgements(ctx)
	if err != nil || len(got2) != 1 {
		t.Fatalf("Acknowledgements tras upsert = %+v err=%v, want seguir en 1", got2, err)
	}
	if got2[0].Note == nil || *got2[0].Note != note2 || !got2[0].At.Equal(at2) {
		t.Fatalf("Acknowledgements[0] tras upsert = %+v, want la nota/hora nuevas", got2[0])
	}

	if err := s.RemoveAcknowledged(ctx, "RISKY_SERVICE|dev:7|554"); err != nil {
		t.Fatalf("RemoveAcknowledged: %v", err)
	}
	got3, err := s.Acknowledgements(ctx)
	if err != nil || len(got3) != 0 {
		t.Fatalf("Acknowledgements tras deshacer = %+v err=%v, want vacío", got3, err)
	}
	// Deshacer algo que no existía no debe fallar.
	if err := s.RemoveAcknowledged(ctx, "no-existe"); err != nil {
		t.Fatalf("RemoveAcknowledged (inexistente): %v", err)
	}
}

// --- PortMappingNotifyKeysSeen: la memoria "ya visto" a partir de lo que
// ya está en port_mappings (Fase 5.1, ver docs/plan.md). ---

// Simula los 3 días de HyperTorrent pidiendo un puerto interior distinto
// en cada sesión (lo que generó las 24 alertas reales): aunque cada fila
// de port_mappings tenga un internal_port distinto, PortMappingNotifyKeysSeen
// debe colapsarlas en la MISMA clave (equipo + programa normalizado), para
// que al desplegar el agrupado no se vuelva a avisar lo de esos días.
func TestPortMappingNotifyKeysSeen_ColapsaHistorialPorPrograma(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	netKey := string(testNet.Key)

	sesiones := []core.PortMapping{
		{Protocol: "TCP", ExternalPort: 51001, InternalIP: "192.168.0.105", InternalPort: 54321, Description: "HyperTorrent 3.2.5", Source: "upnp"},
		{Protocol: "UDP", ExternalPort: 51001, InternalIP: "192.168.0.105", InternalPort: 54321, Description: "HyperTorrent 3.2.5", Source: "upnp"},
	}
	if err := s.ReconcilePortMappings(ctx, netKey, sesiones, securityT0); err != nil {
		t.Fatalf("ReconcilePortMappings (sesión 1): %v", err)
	}
	// Otra sesión, mismo programa, puerto interior DISTINTO: en la tabla
	// queda como otra fila (distinta por internal_ip+protocol+external_port
	// también distinto, o reemplazando active=0 la anterior si coincide en
	// clave) — lo que importa es que port_mappings termine con AL MENOS
	// una fila de "hypertorrent" en 192.168.0.105.
	sesion2 := []core.PortMapping{
		{Protocol: "TCP", ExternalPort: 55102, InternalIP: "192.168.0.105", InternalPort: 60123, Description: "HyperTorrent 3.2.5", Source: "upnp"},
	}
	if err := s.ReconcilePortMappings(ctx, netKey, sesion2, securityT0.Add(time.Hour)); err != nil {
		t.Fatalf("ReconcilePortMappings (sesión 2): %v", err)
	}

	seenKeys, err := s.PortMappingNotifyKeysSeen(ctx, netKey)
	if err != nil {
		t.Fatalf("PortMappingNotifyKeysSeen: %v", err)
	}
	want := alerts.PortMappingNotifyKey{InternalIP: "192.168.0.105", Program: "hypertorrent"}
	if !seenKeys[want] {
		t.Fatalf("seenKeys = %+v, want que contenga %+v (colapsado por programa)", seenKeys, want)
	}

	// Con esa memoria, una redirección NUEVA (otra sesión, otro puerto
	// interior, mismo programa) ya no debe avisar: justo lo que evita la
	// tormenta retroactiva al desplegar el agrupado.
	sesion3 := core.PortMapping{Protocol: "TCP", ExternalPort: 60001, InternalIP: "192.168.0.105", InternalPort: 61000, Description: "HyperTorrent 3.2.5", Source: "upnp"}
	got := alerts.EvaluateNewPortMappings(seenKeys, []core.PortMapping{sesion3}, true, netKey, securityT0.Add(2*time.Hour), alerts.DefaultSettings())
	if len(got) != 0 {
		t.Fatalf("EvaluateNewPortMappings = %+v, want 0 (ya visto por programa, aunque el puerto interior sea nuevo)", got)
	}
}
