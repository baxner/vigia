package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/baxner/vigia/core"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRun_DisparaEscaneoInicialYLuegoPorIntervalo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	recorded := make(chan time.Time, 10)

	scan := func(ctx context.Context) (*core.ScanResult, error) {
		calls.Add(1)
		return &core.ScanResult{}, nil
	}
	record := func(ctx context.Context, res *core.ScanResult, at time.Time) { recorded <- at }

	s := New(ctx, scan, record, 20*time.Millisecond, discardLogger())
	go s.Run()

	select {
	case <-recorded: // escaneo al arrancar
	case <-time.After(2 * time.Second):
		t.Fatal("no llegó el escaneo inicial")
	}

	select {
	case <-recorded: // al menos uno más por el ticker
	case <-time.After(2 * time.Second):
		t.Fatal("no llegó un segundo escaneo por el intervalo")
	}

	cancel()
	if calls.Load() < 2 {
		t.Fatalf("calls = %d, want >= 2", calls.Load())
	}
}

func TestRun_SeDetieneAlCancelarElContexto(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	scan := func(ctx context.Context) (*core.ScanResult, error) { return &core.ScanResult{}, nil }

	s := New(ctx, scan, nil, time.Millisecond, discardLogger())
	done := make(chan struct{})
	go func() { s.Run(); close(done) }()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run no volvió tras cancelar el contexto")
	}
}

func TestTriggerNow_RechazaSiYaHayUnoEnCursoYVuelveAAceptarAlTerminar(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	block := make(chan struct{})
	started := make(chan struct{}, 1)
	scan := func(ctx context.Context) (*core.ScanResult, error) {
		started <- struct{}{}
		<-block
		return &core.ScanResult{}, nil
	}
	s := New(ctx, scan, nil, time.Hour, discardLogger())

	if !s.TriggerNow() {
		t.Fatal("el primer TriggerNow debería aceptar")
	}
	<-started

	if s.TriggerNow() {
		t.Fatal("el segundo TriggerNow debería rechazar: ya hay uno en curso")
	}

	close(block)

	deadline := time.After(2 * time.Second)
	for !s.TriggerNow() {
		select {
		case <-deadline:
			t.Fatal("TriggerNow nunca volvió a aceptar tras terminar el primero")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestExecute_ErrorNoLlamaRecordNiActualizaLastScanAt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recordCalled := make(chan struct{}, 1)
	scan := func(ctx context.Context) (*core.ScanResult, error) { return nil, errors.New("boom") }
	record := func(ctx context.Context, res *core.ScanResult, at time.Time) { recordCalled <- struct{}{} }

	s := New(ctx, scan, record, time.Hour, discardLogger())
	if !s.TriggerNow() {
		t.Fatal("TriggerNow debería aceptar")
	}

	select {
	case <-recordCalled:
		t.Fatal("record no debería llamarse si el escaneo falló")
	case <-time.After(200 * time.Millisecond):
	}

	if !s.LastScanAt().IsZero() {
		t.Fatalf("LastScanAt() = %v, want cero", s.LastScanAt())
	}
	if !s.TriggerNow() {
		t.Fatal("debería poder reintentar: el cupo se libera aunque el escaneo haya fallado")
	}
}

func TestExecute_LastScanAtQuedaEnElInicioDelEscaneo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recorded := make(chan time.Time, 1)
	scan := func(ctx context.Context) (*core.ScanResult, error) {
		time.Sleep(20 * time.Millisecond) // el escaneo "tarda": at no debe ser el de cuando termina
		return &core.ScanResult{}, nil
	}
	record := func(ctx context.Context, res *core.ScanResult, at time.Time) { recorded <- at }

	before := time.Now().UTC()
	s := New(ctx, scan, record, time.Hour, discardLogger())
	s.TriggerNow()

	var at time.Time
	select {
	case at = <-recorded:
	case <-time.After(2 * time.Second):
		t.Fatal("no llegó record")
	}
	after := time.Now().UTC()

	if at.Before(before) || at.After(after) {
		t.Fatalf("at = %v, want entre %v y %v", at, before, after)
	}
	if !s.LastScanAt().Equal(at) {
		t.Fatalf("LastScanAt() = %v, want %v", s.LastScanAt(), at)
	}
}

// --- checkWake (ver docs/agente-api.md, "Suspensión") ---

func TestCheckWake_PrimeraLlamadaSoloFijaLaBase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	scan := func(ctx context.Context) (*core.ScanResult, error) { calls.Add(1); return &core.ScanResult{}, nil }
	s := New(ctx, scan, nil, 5*time.Minute, discardLogger())

	s.checkWake(time.Now())
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("calls = %d, want 0 (la primera llamada no dispara nada)", calls.Load())
	}
}

func TestCheckWake_HuecoNormalNoDisparaEscaneo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	scan := func(ctx context.Context) (*core.ScanResult, error) { calls.Add(1); return &core.ScanResult{}, nil }
	s := New(ctx, scan, nil, 5*time.Minute, discardLogger())

	base := time.Now()
	s.checkWake(base)
	s.checkWake(base.Add(wakePollInterval)) // hueco normal, de verdad entre dos vueltas del ticker.
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("calls = %d, want 0 (hueco normal, no suspensión)", calls.Load())
	}
}

func TestCheckWake_HuecoGrandeDisparaEscaneoInmediato(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	scan := func(ctx context.Context) (*core.ScanResult, error) { calls.Add(1); return &core.ScanResult{}, nil }
	s := New(ctx, scan, nil, 5*time.Minute, discardLogger())

	base := time.Now()
	s.checkWake(base)
	s.checkWake(base.Add(2 * time.Hour)) // >> 2×5min: sospecha suspensión.

	deadline := time.After(2 * time.Second)
	for calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("checkWake no disparó el escaneo tras el hueco grande")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestStartAsync_RecuperaPanicYLiberaElCupo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scan := func(ctx context.Context) (*core.ScanResult, error) { panic("algo salió mal") }
	s := New(ctx, scan, nil, time.Hour, discardLogger())

	if !s.TriggerNow() {
		t.Fatal("TriggerNow debería aceptar")
	}

	deadline := time.After(2 * time.Second)
	for !s.TriggerNow() {
		select {
		case <-deadline:
			t.Fatal("el cupo nunca se liberó tras el panic")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
