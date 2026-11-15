package store

import (
	"context"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/internet"
)

func sampleAgg(at time.Time, routerMs float64) internet.Agg {
	return internet.Agg{
		At:       at,
		Router:   internet.SegmentStats{Sent: 2, Lost: 0, MinMs: routerMs, AvgMs: routerMs, MaxMs: routerMs},
		Modem:    internet.SegmentStats{Sent: 2, Lost: 1, MinMs: 4, AvgMs: 4, MaxMs: 4},
		Internet: internet.SegmentStats{Sent: 2, Lost: 0, MinMs: 18, AvgMs: 18, MaxMs: 18},
		DNS:      internet.SegmentStats{Sent: 2, Lost: 0, MinMs: 10, AvgMs: 10, MaxMs: 10},
	}
}

func TestRecordMinuteAgg_GuardaYListaPorRango(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	// Reciente de verdad (no una fecha fija): RecordMinuteAgg poda contra
	// time.Now() real en cada escritura (ver el comentario de
	// recordInternetAgg), así que una fecha fija del pasado se podaría de
	// inmediato.
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Minute)

	for i := 0; i < 3; i++ {
		if err := s.RecordMinuteAgg(ctx, sampleAgg(base.Add(time.Duration(i)*time.Minute), float64(i+1))); err != nil {
			t.Fatalf("RecordMinuteAgg: %v", err)
		}
	}

	got, err := s.MinuteSeries(ctx, base)
	if err != nil {
		t.Fatalf("MinuteSeries: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if !got[0].At.Equal(base) {
		t.Fatalf("got[0].At = %v, want %v", got[0].At, base)
	}
	if got[1].Router.AvgMs != 2 {
		t.Fatalf("got[1].Router.AvgMs = %v, want 2", got[1].Router.AvgMs)
	}
	if got[0].Modem.LossPct() != 50 {
		t.Fatalf("got[0].Modem.LossPct() = %v, want 50", got[0].Modem.LossPct())
	}

	// Filtrado: since después del primero debe excluirlo.
	got2, err := s.MinuteSeries(ctx, base.Add(30*time.Second))
	if err != nil {
		t.Fatalf("MinuteSeries: %v", err)
	}
	if len(got2) != 2 {
		t.Fatalf("len(got2) = %d, want 2", len(got2))
	}
}

func TestRecordMinuteAgg_PodaLoMasViejoQue7Dias(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	old := now.Add(-8 * 24 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	if err := s.RecordMinuteAgg(ctx, sampleAgg(old, 1)); err != nil {
		t.Fatalf("RecordMinuteAgg(old): %v", err)
	}
	if err := s.RecordMinuteAgg(ctx, sampleAgg(recent, 2)); err != nil {
		t.Fatalf("RecordMinuteAgg(recent): %v", err)
	}

	got, err := s.MinuteSeries(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("MinuteSeries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (la de 8 días debió podarse)", len(got))
	}
	if !got[0].At.Equal(recent.Truncate(time.Second)) && got[0].At.Unix() != recent.Unix() {
		t.Fatalf("got[0].At = %v, want %v", got[0].At, recent)
	}
}

func TestRecordHourAgg_RetencionDistintaDeMinuto(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Más vieja que la retención de MINUTO pero más nueva que la de HORA:
	// debe sobrevivir en internet_hours.
	mid := now.Add(-10 * 24 * time.Hour)
	if err := s.RecordHourAgg(ctx, sampleAgg(mid, 5)); err != nil {
		t.Fatalf("RecordHourAgg: %v", err)
	}
	got, err := s.HourSeries(ctx, now.Add(-100*24*time.Hour))
	if err != nil {
		t.Fatalf("HourSeries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (90 días no debió podarla)", len(got))
	}
}

func TestOutages_AbrirCerrarYListar(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	// Sin caída abierta todavía.
	if _, _, err := s.OpenOutage(ctx); err != ErrNotFound {
		t.Fatalf("OpenOutage sin filas: err=%v, want ErrNotFound", err)
	}

	id, err := s.InsertOutageOpen(ctx, internet.Outage{Start: start, Blame: internet.BlameISP})
	if err != nil {
		t.Fatalf("InsertOutageOpen: %v", err)
	}
	if id == 0 {
		t.Fatal("id = 0")
	}

	gotID, o, err := s.OpenOutage(ctx)
	if err != nil {
		t.Fatalf("OpenOutage: %v", err)
	}
	if gotID != id {
		t.Fatalf("gotID = %d, want %d", gotID, id)
	}
	if o.Blame != internet.BlameISP || !o.Start.Equal(start) {
		t.Fatalf("o = %+v", o)
	}

	end := start.Add(3 * time.Minute)
	if err := s.CloseOutage(ctx, id, end); err != nil {
		t.Fatalf("CloseOutage: %v", err)
	}

	if _, _, err := s.OpenOutage(ctx); err != ErrNotFound {
		t.Fatalf("tras cerrar: err=%v, want ErrNotFound", err)
	}

	rows, err := s.RecentOutages(ctx, start.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("RecentOutages: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].End == nil || !rows[0].End.Equal(end) {
		t.Fatalf("rows[0].End = %v, want %v", rows[0].End, end)
	}
	if rows[0].DurationSec != 180 {
		t.Fatalf("DurationSec = %v, want 180", rows[0].DurationSec)
	}
}

func TestRecentOutages_FiltraPorVentanaDeDias(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	oldStart := now.Add(-40 * 24 * time.Hour)
	oldEnd := oldStart.Add(time.Minute)
	recentStart := now.Add(-2 * time.Hour)

	oldID, _ := s.InsertOutageOpen(ctx, internet.Outage{Start: oldStart, Blame: internet.BlameLAN})
	s.CloseOutage(ctx, oldID, oldEnd)
	recentID, _ := s.InsertOutageOpen(ctx, internet.Outage{Start: recentStart, Blame: internet.BlameModem})
	s.CloseOutage(ctx, recentID, recentStart.Add(time.Minute))

	rows, err := s.RecentOutages(ctx, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("RecentOutages: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 (la de 40 días queda fuera de la ventana de 30)", len(rows))
	}
	if rows[0].Blame != internet.BlameModem {
		t.Fatalf("rows[0].Blame = %v, want modem", rows[0].Blame)
	}
}

func TestSpeedTests_RegistrarYListar(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if _, err := s.RecordSpeedTest(ctx, internet.SpeedTestResult{At: at, DownMbps: 95.5, UpMbps: 20.1, LatencyMs: 12, LoadedLatencyMs: 45}); err != nil {
		t.Fatalf("RecordSpeedTest: %v", err)
	}
	errMsg := "timeout"
	if _, err := s.RecordSpeedTest(ctx, internet.SpeedTestResult{At: at.Add(time.Hour), Error: &errMsg}); err != nil {
		t.Fatalf("RecordSpeedTest(error): %v", err)
	}

	rows, err := s.RecentSpeedTests(ctx, 0)
	if err != nil {
		t.Fatalf("RecentSpeedTests: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	// La más nueva primero.
	if rows[0].Error == nil || *rows[0].Error != "timeout" {
		t.Fatalf("rows[0].Error = %v, want timeout", rows[0].Error)
	}
	if rows[1].DownMbps != 95.5 {
		t.Fatalf("rows[1].DownMbps = %v, want 95.5", rows[1].DownMbps)
	}
}

func TestInternetSettings_DefaultYPersistencia(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	got, err := s.InternetSettings(ctx)
	if err != nil {
		t.Fatalf("InternetSettings: %v", err)
	}
	if got != internet.DefaultSettings() {
		t.Fatalf("got = %+v, want default", got)
	}

	if err := s.SetInternetSettings(ctx, internet.Settings{SpeedTestsPerDay: 4}); err != nil {
		t.Fatalf("SetInternetSettings: %v", err)
	}
	got2, err := s.InternetSettings(ctx)
	if err != nil {
		t.Fatalf("InternetSettings: %v", err)
	}
	if got2.SpeedTestsPerDay != 4 {
		t.Fatalf("SpeedTestsPerDay = %d, want 4", got2.SpeedTestsPerDay)
	}
}

func TestAlertSettings_IncluyeInternetDownRestored(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	got, err := s.AlertSettings(ctx)
	if err != nil {
		t.Fatalf("AlertSettings: %v", err)
	}
	if !got.InternetDown || !got.InternetRestored {
		t.Fatalf("got = %+v, want ambas en true por defecto", got)
	}

	got.InternetDown = false
	if err := s.SetAlertSettings(ctx, got); err != nil {
		t.Fatalf("SetAlertSettings: %v", err)
	}
	got2, err := s.AlertSettings(ctx)
	if err != nil {
		t.Fatalf("AlertSettings: %v", err)
	}
	if got2.InternetDown {
		t.Fatal("InternetDown debió quedar en false")
	}
	if !got2.InternetRestored {
		t.Fatal("InternetRestored no debió tocarse")
	}
}
