package main

import (
	"context"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/internet"
)

func segOK(ms float64) *internet.Segment { s := internet.Segment{MS: ms, OK: true}; return &s }

func okSample(at time.Time) internet.Sample {
	return internet.Sample{
		At: at, Router: segOK(2), Modem: segOK(4),
		Internet: internet.Segment{MS: 18, OK: true}, DNS: internet.Segment{MS: 10, OK: true},
	}
}

func downSample(at time.Time) internet.Sample {
	return internet.Sample{
		At: at, Router: segOK(2), Modem: segOK(4),
		Internet: internet.Segment{OK: false}, DNS: internet.Segment{OK: false},
	}
}

func newTestMonitor(t *testing.T) (*internetMonitor, *testAgentEnv) {
	t.Helper()
	env := newTestAgentEnv(t)
	netInfo := func() (string, string) { return testInnerNetKey, "192.168.0.4" }
	// measure nunca se llama en estos tests: process() se alimenta a mano
	// (ver TestInternetMonitor_*), measure solo lo usa tick()/Run().
	m := newInternetMonitor(env.store, env.hub, env.sender, env.id, nil, netInfo, env.log)
	return m, env
}

func TestInternetMonitor_AbreYCierraCaidaConAlertas(t *testing.T) {
	m, env := newTestMonitor(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	m.process(ctx, okSample(base), internet.IPv6None)
	m.process(ctx, downSample(base.Add(30*time.Second)), internet.IPv6None)
	if m.OutageOpen() {
		t.Fatal("1 falla no debe abrir la caída todavía")
	}
	m.process(ctx, downSample(base.Add(60*time.Second)), internet.IPv6None)
	if !m.OutageOpen() {
		t.Fatal("2 fallas seguidas deben abrir la caída")
	}

	state, since, _, _ := m.InternetNow()
	if state != internet.StateDown {
		t.Fatalf("state = %v, want down", state)
	}
	if !since.Equal(base.Add(30 * time.Second)) {
		t.Fatalf("since = %v, want %v (la PRIMERA falla)", since, base.Add(30*time.Second))
	}

	// La caída debe haber quedado persistida y abierta.
	id, o, err := env.store.OpenOutage(ctx)
	if err != nil {
		t.Fatalf("OpenOutage: %v", err)
	}
	if id == 0 || o.Blame != internet.BlameISP {
		t.Fatalf("id/o = %d/%+v", id, o)
	}

	// Y debe haber una alerta INTERNET_DOWN.
	alertsGot, err := env.store.Alerts(ctx, 0, 0)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if len(alertsGot) != 1 || string(alertsGot[0].Type) != "INTERNET_DOWN" {
		t.Fatalf("alerts = %+v, want 1 INTERNET_DOWN", alertsGot)
	}

	// Recuperación: 1 acierto no cierra, 2 seguidos sí.
	m.process(ctx, okSample(base.Add(90*time.Second)), internet.IPv6None)
	if !m.OutageOpen() {
		t.Fatal("1 acierto no debe cerrar todavía")
	}
	m.process(ctx, okSample(base.Add(120*time.Second)), internet.IPv6None)
	if m.OutageOpen() {
		t.Fatal("2 aciertos seguidos deben cerrar la caída")
	}

	if _, _, err := env.store.OpenOutage(ctx); err == nil {
		t.Fatal("ya no debería haber caída abierta en el store")
	}

	alertsGot2, err := env.store.Alerts(ctx, 0, 0)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if len(alertsGot2) != 2 {
		t.Fatalf("alerts = %+v, want 2 (DOWN + RESTORED)", alertsGot2)
	}
	if string(alertsGot2[1].Type) != "INTERNET_RESTORED" {
		t.Fatalf("alerts[1].Type = %v, want INTERNET_RESTORED", alertsGot2[1].Type)
	}

	// Justo al cerrar la caída, el ÚLTIMO minuto cerrado (12:01, con una
	// falla y un acierto) todavía arrastra 50% de pérdida: EvaluateState
	// lo marca "degraded" aunque la caída ya se cerró — correcto, no es
	// un bug (ver docs/agente-api.md: "degraded: pérdida > 5%"). Dos
	// muestras limpias más, hasta que cierre un minuto SIN pérdida,
	// deben volver el estado a "up".
	state2, _, _, _ := m.InternetNow()
	if state2 != internet.StateDegraded {
		t.Fatalf("state = %v, want degraded (el minuto 12:01 todavía tiene 50%% de pérdida)", state2)
	}
	m.process(ctx, okSample(base.Add(150*time.Second)), internet.IPv6None)
	m.process(ctx, okSample(base.Add(180*time.Second)), internet.IPv6None) // cierra el minuto 12:02, sin pérdida.

	state3, _, _, _ := m.InternetNow()
	if state3 != internet.StateUp {
		t.Fatalf("state = %v, want up", state3)
	}
}

func TestInternetMonitor_AgregaPorMinutoYLoGuarda(t *testing.T) {
	m, env := newTestMonitor(t)
	ctx := context.Background()
	// Reciente de verdad: RecordMinuteAgg poda contra time.Now() real en
	// cada escritura (ver agent/store.recordInternetAgg), así que una
	// fecha fija del pasado se podaría de inmediato.
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Minute)

	m.process(ctx, okSample(base), internet.IPv6OK)
	m.process(ctx, okSample(base.Add(30*time.Second)), internet.IPv6OK)
	// Muestra del minuto SIGUIENTE: cierra la ventana del minuto 0.
	m.process(ctx, okSample(base.Add(60*time.Second)), internet.IPv6OK)

	rows, err := env.store.MinuteSeries(ctx, base.Add(-time.Minute))
	if err != nil {
		t.Fatalf("MinuteSeries: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Router.Sent != 2 || rows[0].Router.AvgMs != 2 {
		t.Fatalf("rows[0].Router = %+v", rows[0].Router)
	}

	_, _, now, ipv6 := m.InternetNow()
	if ipv6 != internet.IPv6OK {
		t.Fatalf("ipv6 = %v, want ok", ipv6)
	}
	if now.Internet.MS != 18 {
		t.Fatalf("now.Internet.MS = %v, want 18 (la ÚLTIMA muestra)", now.Internet.MS)
	}
}

func TestInternetMonitor_DegradadoPorPerdida(t *testing.T) {
	m, _ := newTestMonitor(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// 1 falla de 20 muestras en el mismo minuto (5% exacto, no > 5): no
	// degrada. Luego una más (2 de 21, > 5%... en realidad verificamos con
	// una ventana simple de 2 muestras: 1 de 2 = 50% de pérdida en el
	// minuto que se cierra).
	m.process(ctx, downSample(base), internet.IPv6None)
	m.process(ctx, okSample(base.Add(30*time.Second)), internet.IPv6None)
	// Cierra el minuto anterior (1 falla de 2 = 50% de pérdida) y evalúa
	// el estado con esa pérdida.
	m.process(ctx, okSample(base.Add(60*time.Second)), internet.IPv6None)

	state, _, _, _ := m.InternetNow()
	if state != internet.StateDegraded {
		t.Fatalf("state = %v, want degraded (50%% de pérdida el minuto pasado)", state)
	}
}

func TestInternetMonitor_ResumeCaidaAbiertaAlArrancar(t *testing.T) {
	env := newTestAgentEnv(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	if _, err := env.store.InsertOutageOpen(ctx, internet.Outage{Start: start, Blame: internet.BlameLAN}); err != nil {
		t.Fatalf("InsertOutageOpen: %v", err)
	}

	netInfo := func() (string, string) { return testInnerNetKey, "192.168.0.4" }
	m := newInternetMonitor(env.store, env.hub, env.sender, env.id, nil, netInfo, env.log)

	if !m.OutageOpen() {
		t.Fatal("debió resumir la caída abierta")
	}
	state, since, _, _ := m.InternetNow()
	if state != internet.StateDown {
		t.Fatalf("state = %v, want down", state)
	}
	if !since.Equal(start) {
		t.Fatalf("since = %v, want %v", since, start)
	}
}

// TestInternetMonitor_SuspensionNoFabricaCaida: una falla, un hueco
// grande (la máquina se durmió) y otra falla suelta y no relacionada NO
// deben combinarse en una caída que parezca haber durado todo el hueco
// (ver agent/internet.OutageDetector.Reset y el comentario de process en
// internet.go, "Suspensión").
func TestInternetMonitor_SuspensionNoFabricaCaida(t *testing.T) {
	m, env := newTestMonitor(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	m.process(ctx, downSample(base), internet.IPv6None) // 1 falla, sin confirmar.
	if m.OutageOpen() {
		t.Fatal("1 falla no debe abrir nada")
	}

	// La máquina se duerme 2 horas; al despertar, una falla suelta.
	m.process(ctx, downSample(base.Add(2*time.Hour)), internet.IPv6None)
	if m.OutageOpen() {
		t.Fatal("tras el hueco, 1 sola falla (ya reiniciada la racha) no debe abrir nada")
	}
	if _, _, err := env.store.OpenOutage(ctx); err == nil {
		t.Fatal("no debería haberse persistido ninguna caída")
	}

	// Una SEGUNDA falla de verdad, ya despierto, sí abre la caída —con
	// Start en la primera falla DESPUÉS del hueco, no en la de antes.
	m.process(ctx, downSample(base.Add(2*time.Hour).Add(30*time.Second)), internet.IPv6None)
	if !m.OutageOpen() {
		t.Fatal("2 fallas de verdad tras el hueco deben abrir la caída")
	}
	_, since, _, _ := m.InternetNow()
	if !since.Equal(base.Add(2 * time.Hour)) {
		t.Fatalf("since = %v, want %v (la primera falla DESPUÉS del hueco)", since, base.Add(2*time.Hour))
	}
}
