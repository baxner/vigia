package store

import (
	"context"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/core"
)

func TestMigration2_UserVersionEsDos(t *testing.T) {
	s := openTestStore(t)
	var version int
	if err := s.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != len(migrations) {
		t.Fatalf("user_version = %d, want %d (len(migrations))", version, len(migrations))
	}
}

func TestInsertAlert_CreaYDedupeaPorMismaClave(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	net := NetworkInfo{Key: "net1", Subnet: "192.168.0.0/24", Gateway: "gw"}
	events, err := s.RecordScan(ctx, net, []core.Device{{IP: "192.168.0.24", MAC: "aa:bb:cc:dd:ee:02"}}, at)
	if err != nil || len(events) != 1 {
		t.Fatalf("RecordScan: events=%v err=%v", events, err)
	}
	id := events[0].Event.DeviceID
	a := alerts.Alert{Type: alerts.NewDevice, Severity: alerts.High, At: at, Title: "Equipo nuevo en tu red",
		Body: "OnePlus (192.168.0.24) se conectó por primera vez.", DeviceID: &id, Network: "net1",
		DedupKey: "NEW_DEVICE|test|" + at.Format(time.RFC3339Nano)}

	persisted, created, err := s.InsertAlert(ctx, a)
	if err != nil {
		t.Fatalf("InsertAlert: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true (primera vez)")
	}
	if persisted.ID == 0 {
		t.Fatal("esperaba un id != 0")
	}
	if persisted.Type != alerts.NewDevice || persisted.Title != a.Title || persisted.Body != a.Body {
		t.Fatalf("persisted = %+v, no coincide con a", persisted)
	}
	if persisted.DeviceID == nil || *persisted.DeviceID != id {
		t.Fatalf("deviceId = %v, want %d", persisted.DeviceID, id)
	}
	if persisted.Read {
		t.Fatal("read = true, want false (recién creada)")
	}

	// Reevaluar el MISMO evento (misma DedupKey): no duplica.
	again, created2, err := s.InsertAlert(ctx, a)
	if err != nil {
		t.Fatalf("segundo InsertAlert: %v", err)
	}
	if created2 {
		t.Fatal("created2 = true, want false: la misma DedupKey no debe duplicar la alerta")
	}
	if again.ID != 0 {
		t.Fatalf("again = %+v, want zero-value cuando created=false", again)
	}

	all, err := s.Alerts(ctx, 0, 100)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("Alerts = %d filas, want exactamente 1 (idempotencia)", len(all))
	}
}

func TestInsertAlert_AgentBlindSinDeviceID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	a := alerts.Alert{Type: alerts.AgentBlind, Severity: alerts.Medium, At: at, Title: "Vigía dejó de ver tu red",
		Body: "sin permiso de red local", Network: "net1", DedupKey: "AGENT_BLIND|net1|" + at.Format(time.RFC3339Nano)}

	persisted, created, err := s.InsertAlert(ctx, a)
	if err != nil || !created {
		t.Fatalf("InsertAlert: created=%v err=%v", created, err)
	}
	if persisted.DeviceID != nil {
		t.Fatalf("deviceId = %v, want nil (AGENT_BLIND)", persisted.DeviceID)
	}
	if persisted.Network != "net1" {
		t.Fatalf("network = %q, want net1", persisted.Network)
	}
}

func TestInsertAlert_SinDedupKeyEsError(t *testing.T) {
	s := openTestStore(t)
	if _, _, err := s.InsertAlert(context.Background(), alerts.Alert{Type: alerts.NewDevice}); err == nil {
		t.Fatal("esperaba un error: DedupKey vacío")
	}
}

func TestAlerts_AfterYLimit(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	var ids []int64
	for i := 0; i < 3; i++ {
		at := t0.Add(time.Duration(i) * time.Minute)
		a := alerts.Alert{Type: alerts.AgentBlind, Severity: alerts.Medium, At: at, Title: "t", Body: "b",
			Network: "net1", DedupKey: "k" + string(rune('A'+i))}
		p, created, err := s.InsertAlert(ctx, a)
		if err != nil || !created {
			t.Fatalf("InsertAlert %d: created=%v err=%v", i, created, err)
		}
		ids = append(ids, p.ID)
	}

	all, err := s.Alerts(ctx, 0, 100)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}
	for i, a := range all {
		if a.ID != ids[i] {
			t.Fatalf("orden = %v, want %v (de la más vieja a la más nueva)", idsOf(all), ids)
		}
	}

	after, err := s.Alerts(ctx, ids[0], 100)
	if err != nil {
		t.Fatalf("Alerts after: %v", err)
	}
	if len(after) != 2 || after[0].ID != ids[1] {
		t.Fatalf("after=%d: got %v, want [%d %d]", ids[0], idsOf(after), ids[1], ids[2])
	}

	limited, err := s.Alerts(ctx, 0, 1)
	if err != nil {
		t.Fatalf("Alerts limit=1: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != ids[0] {
		t.Fatalf("limit=1: got %v, want [%d]", idsOf(limited), ids[0])
	}
}

func idsOf(a []Alert) []int64 {
	out := make([]int64, len(a))
	for i, x := range a {
		out[i] = x.ID
	}
	return out
}

func TestMarkAlertsRead(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	a1, _, _ := s.InsertAlert(ctx, alerts.Alert{Type: alerts.AgentBlind, At: t0, Title: "t", Body: "b", DedupKey: "k1"})
	a2, _, _ := s.InsertAlert(ctx, alerts.Alert{Type: alerts.AgentBlind, At: t0.Add(time.Minute), Title: "t", Body: "b", DedupKey: "k2"})

	if err := s.MarkAlertsRead(ctx, a1.ID); err != nil {
		t.Fatalf("MarkAlertsRead: %v", err)
	}

	all, err := s.Alerts(ctx, 0, 100)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	byID := map[int64]Alert{}
	for _, a := range all {
		byID[a.ID] = a
	}
	if !byID[a1.ID].Read {
		t.Fatal("a1.Read = false, want true (upTo lo incluye)")
	}
	if byID[a2.ID].Read {
		t.Fatal("a2.Read = true, want false (upTo no lo alcanza)")
	}
}

func TestAlertSettings_DefaultYReemplazo(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	got, err := s.AlertSettings(ctx)
	if err != nil {
		t.Fatalf("AlertSettings: %v", err)
	}
	if got != alerts.DefaultSettings() {
		t.Fatalf("AlertSettings recién migrado = %+v, want todo encendido", got)
	}

	custom := alerts.Settings{NewDevice: false, ImportantOffline: true, ImportantOnline: false, AgentBlind: true, AgentRecovered: false}
	if err := s.SetAlertSettings(ctx, custom); err != nil {
		t.Fatalf("SetAlertSettings: %v", err)
	}
	got2, err := s.AlertSettings(ctx)
	if err != nil {
		t.Fatalf("AlertSettings tras set: %v", err)
	}
	if got2 != custom {
		t.Fatalf("AlertSettings = %+v, want %+v", got2, custom)
	}
}

// Olvidar un equipo no debe borrar su historial de alertas, solo
// desvincularlas (ON DELETE SET NULL en migration2, no CASCADE).
func TestInsertAlert_DeviceIDQuedaNilSiElEquipoSeOlvida(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	net := NetworkInfo{Key: "gw|192.168.0.0/24", Subnet: "192.168.0.0/24", Gateway: "gw"}
	events, err := s.RecordScan(ctx, net, []core.Device{{IP: "192.168.0.10", MAC: "aa:bb:cc:dd:ee:01"}}, time.Now().UTC())
	if err != nil || len(events) != 1 {
		t.Fatalf("RecordScan: events=%v err=%v", events, err)
	}
	deviceID := events[0].Event.DeviceID

	a := alerts.Alert{Type: alerts.NewDevice, Severity: alerts.High, At: time.Now().UTC(), Title: "t", Body: "b",
		DeviceID: &deviceID, Network: string(net.Key), DedupKey: "k-forget"}
	persisted, created, err := s.InsertAlert(ctx, a)
	if err != nil || !created {
		t.Fatalf("InsertAlert: created=%v err=%v", created, err)
	}

	if err := s.Forget(ctx, deviceID); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	all, err := s.Alerts(ctx, 0, 100)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("len(all) = %d, want 1 (la alerta sobrevive)", len(all))
	}
	if all[0].ID != persisted.ID {
		t.Fatalf("id = %d, want %d", all[0].ID, persisted.ID)
	}
	if all[0].DeviceID != nil {
		t.Fatalf("deviceId = %v, want nil tras olvidar el equipo", all[0].DeviceID)
	}
}
