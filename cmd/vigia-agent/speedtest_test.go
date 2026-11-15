package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/internet"
)

// fakeCloudflareServer imita los dos endpoints que usa newCloudflareSpeedTest
// (ver speedtest.go): __down?bytes=N devuelve N bytes cualquiera,
// __up lee y descarta el cuerpo. Nunca sale a internet — httptest.Server
// escucha en loopback.
func fakeCloudflareServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/__down", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("bytes"))
		w.WriteHeader(http.StatusOK)
		buf := make([]byte, 64*1024)
		remaining := n
		for remaining > 0 {
			chunk := len(buf)
			if remaining < chunk {
				chunk = remaining
			}
			if _, err := w.Write(buf[:chunk]); err != nil {
				return
			}
			remaining -= chunk
		}
	})
	mux.HandleFunc("/__up", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCloudflareSpeedTest_ContraServidorFalso(t *testing.T) {
	srv := fakeCloudflareServer(t)
	run := newCloudflareSpeedTest(&http.Client{}, srv.URL)

	result := run(context.Background())
	if result.Error != nil {
		t.Fatalf("Error = %v, want nil", *result.Error)
	}
	if result.DownMbps <= 0 {
		t.Fatalf("DownMbps = %v, want > 0", result.DownMbps)
	}
	if result.UpMbps <= 0 {
		t.Fatalf("UpMbps = %v, want > 0", result.UpMbps)
	}
	if result.LatencyMs < 0 {
		t.Fatalf("LatencyMs = %v, want >= 0", result.LatencyMs)
	}
	if result.At.IsZero() {
		t.Fatal("At vacío")
	}
}

func TestCloudflareSpeedTest_ServidorCaidoDaError(t *testing.T) {
	srv := fakeCloudflareServer(t)
	srv.Close() // ya no escucha: cualquier intento falla.

	run := newCloudflareSpeedTest(&http.Client{Timeout: 2 * time.Second}, srv.URL)
	result := run(context.Background())
	if result.Error == nil {
		t.Fatal("Error = nil, want un error (servidor caído)")
	}
	if result.DownMbps != 0 || result.UpMbps != 0 {
		t.Fatalf("resultado con error no debe traer Mbps: %+v", result)
	}
}

func TestSpeedTestRunner_TriggerNow_409SiYaHayUnaCorriendo(t *testing.T) {
	env := newTestAgentEnv(t)
	started := make(chan struct{})
	release := make(chan struct{})
	run := func(ctx context.Context) internet.SpeedTestResult {
		close(started)
		<-release
		return internet.SpeedTestResult{At: time.Now().UTC(), DownMbps: 1}
	}
	r := newSpeedTestRunner(env.store, func() bool { return false }, run, time.Now().UTC(), time.Hour, env.log)

	if !r.TriggerNow(context.Background()) {
		t.Fatal("primer TriggerNow debió aceptar")
	}
	<-started
	if r.TriggerNow(context.Background()) {
		t.Fatal("segundo TriggerNow con una corriendo debió devolver false (409)")
	}
	if !r.Running() {
		t.Fatal("Running() debió ser true")
	}
	close(release)

	// Espera a que la goroutine termine (Running vuelve a false).
	deadline := time.Now().Add(2 * time.Second)
	for r.Running() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if r.Running() {
		t.Fatal("Running() debió volver a false")
	}

	// Y el resultado debe haber quedado guardado en el store.
	rows, err := env.store.RecentSpeedTests(context.Background(), 0)
	if err != nil {
		t.Fatalf("RecentSpeedTests: %v", err)
	}
	if len(rows) != 1 || rows[0].DownMbps != 1 {
		t.Fatalf("rows = %+v, want 1 fila con DownMbps=1", rows)
	}
}

func TestSpeedTestRunner_MaybeRun_NoCorreDuranteUnaCaida(t *testing.T) {
	env := newTestAgentEnv(t)
	var calls atomic.Int32
	run := func(ctx context.Context) internet.SpeedTestResult {
		calls.Add(1)
		return internet.SpeedTestResult{At: time.Now().UTC(), DownMbps: 1}
	}
	outageOpen := true
	r := newSpeedTestRunner(env.store, func() bool { return outageOpen }, run, time.Now().UTC().Add(-time.Hour), 0, env.log)

	r.maybeRun(context.Background(), time.Now().UTC())
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("no debió correr con una caída abierta")
	}

	outageOpen = false
	r.maybeRun(context.Background(), time.Now().UTC())
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (sin caída, ya debió correr)", calls.Load())
	}
}

func TestSpeedTestRunner_MaybeRun_RespetaSpeedTestsPerDayCero(t *testing.T) {
	env := newTestAgentEnv(t)
	if err := env.store.SetInternetSettings(context.Background(), internet.Settings{SpeedTestsPerDay: 0}); err != nil {
		t.Fatalf("SetInternetSettings: %v", err)
	}
	var calls atomic.Int32
	run := func(ctx context.Context) internet.SpeedTestResult {
		calls.Add(1)
		return internet.SpeedTestResult{At: time.Now().UTC()}
	}
	r := newSpeedTestRunner(env.store, func() bool { return false }, run, time.Now().UTC().Add(-time.Hour), 0, env.log)

	r.maybeRun(context.Background(), time.Now().UTC())
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("con speedTestsPerDay=0 no debió correr nada")
	}
}

func TestSpeedTestRunner_ScheduleNext_RepartidasEnElDia(t *testing.T) {
	env := newTestAgentEnv(t)
	if err := env.store.SetInternetSettings(context.Background(), internet.Settings{SpeedTestsPerDay: 4}); err != nil {
		t.Fatalf("SetInternetSettings: %v", err)
	}
	run := func(ctx context.Context) internet.SpeedTestResult {
		return internet.SpeedTestResult{At: time.Now().UTC()}
	}
	r := newSpeedTestRunner(env.store, func() bool { return false }, run, time.Now().UTC(), 0, env.log)

	before := time.Now().UTC()
	if !r.TriggerNow(context.Background()) {
		t.Fatal("TriggerNow debió aceptar")
	}
	deadline := time.Now().Add(2 * time.Second)
	for r.Running() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	r.mu.Lock()
	next := r.nextAt
	r.mu.Unlock()
	wantMin := before.Add(5 * time.Hour) // 24h/4 = 6h, con margen por debajo.
	wantMax := before.Add(7 * time.Hour)
	if next.Before(wantMin) || next.After(wantMax) {
		t.Fatalf("nextAt = %v, want entre %v y %v (24h/4=6h desde ahora)", next, wantMin, wantMax)
	}
}
