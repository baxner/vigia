package main

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/baxner/vigia/core"
)

func TestNewOuterRingConfig_VacioAutodetectaDespues(t *testing.T) {
	c, err := newOuterRingConfig("", "")
	if err != nil {
		t.Fatalf("newOuterRingConfig: %v", err)
	}
	if _, _, ok := c.current(); ok {
		t.Fatal("current() ok=true antes de observeRouter, want false (todavía no hay nada autodetectado)")
	}
	c.observeRouter(&core.RouterInfo{Reach: "outer_ring", ExternalIP: "192.168.100.2"})
	cidr, gw, ok := c.current()
	if !ok {
		t.Fatal("current() ok=false después de observeRouter con reach outer_ring, want true")
	}
	if cidr != "192.168.100.0/24" || gw != "192.168.100.1" {
		t.Fatalf("cidr/gw = %q/%q, want 192.168.100.0/24 / 192.168.100.1", cidr, gw)
	}
}

func TestOuterRingConfig_ObserveRouter_IgnoraReachNoOuterRing(t *testing.T) {
	c, err := newOuterRingConfig("", "")
	if err != nil {
		t.Fatalf("newOuterRingConfig: %v", err)
	}
	c.observeRouter(&core.RouterInfo{Reach: "internet", ExternalIP: "8.8.8.8"})
	if _, _, ok := c.current(); ok {
		t.Fatal("current() ok=true con reach=internet, want false (IP pública: no hay anillo intermedio)")
	}
	c.observeRouter(&core.RouterInfo{Reach: "unknown"})
	if _, _, ok := c.current(); ok {
		t.Fatal("current() ok=true con reach=unknown, want false")
	}
	c.observeRouter(nil)
	if _, _, ok := c.current(); ok {
		t.Fatal("current() ok=true con info=nil, want false")
	}
}

func TestOuterRingConfig_ObserveRouter_ActualizaSiCambiaLaIPExterna(t *testing.T) {
	c, err := newOuterRingConfig("", "")
	if err != nil {
		t.Fatalf("newOuterRingConfig: %v", err)
	}
	c.observeRouter(&core.RouterInfo{Reach: "outer_ring", ExternalIP: "192.168.100.2"})
	c.observeRouter(&core.RouterInfo{Reach: "outer_ring", ExternalIP: "10.50.9.2"}) // el proveedor reasignó la IP CGNAT
	cidr, gw, ok := c.current()
	if !ok || cidr != "10.50.9.0/24" || gw != "10.50.9.1" {
		t.Fatalf("cidr/gw/ok = %q/%q/%v, want la IP más reciente (10.50.9.x)", cidr, gw, ok)
	}
}

func TestNewOuterRingConfig_Off(t *testing.T) {
	c, err := newOuterRingConfig("off", "")
	if err != nil {
		t.Fatalf("newOuterRingConfig: %v", err)
	}
	if _, _, ok := c.current(); ok {
		t.Fatal("current() ok=true con -outer off, want false")
	}
	c.observeRouter(&core.RouterInfo{Reach: "outer_ring", ExternalIP: "192.168.100.2"})
	if _, _, ok := c.current(); ok {
		t.Fatal("current() ok=true tras observeRouter con -outer off, want seguir false: apagado es apagado")
	}
}

func TestNewOuterRingConfig_FijoSinGateway_UsaPuntoUno(t *testing.T) {
	c, err := newOuterRingConfig("10.1.2.0/24", "")
	if err != nil {
		t.Fatalf("newOuterRingConfig: %v", err)
	}
	cidr, gw, ok := c.current()
	if !ok || cidr != "10.1.2.0/24" || gw != "10.1.2.1" {
		t.Fatalf("cidr/gw/ok = %q/%q/%v, want 10.1.2.0/24 / 10.1.2.1 / true", cidr, gw, ok)
	}
}

func TestNewOuterRingConfig_FijoConGatewayExplicito(t *testing.T) {
	c, err := newOuterRingConfig("10.1.2.0/24", "10.1.2.254")
	if err != nil {
		t.Fatalf("newOuterRingConfig: %v", err)
	}
	cidr, gw, ok := c.current()
	if !ok || cidr != "10.1.2.0/24" || gw != "10.1.2.254" {
		t.Fatalf("cidr/gw/ok = %q/%q/%v, want gateway explícito", cidr, gw, ok)
	}
}

func TestNewOuterRingConfig_Fijo_IgnoraObserveRouter(t *testing.T) {
	c, err := newOuterRingConfig("10.1.2.0/24", "")
	if err != nil {
		t.Fatalf("newOuterRingConfig: %v", err)
	}
	// Autodetección jamás debe pisar un anillo fijado a mano por flags.
	c.observeRouter(&core.RouterInfo{Reach: "outer_ring", ExternalIP: "192.168.100.2"})
	cidr, gw, ok := c.current()
	if !ok || cidr != "10.1.2.0/24" || gw != "10.1.2.1" {
		t.Fatalf("cidr/gw/ok = %q/%q/%v, want que -outer fijo no cambie", cidr, gw, ok)
	}
}

func TestNewOuterRingConfig_CIDRInvalido(t *testing.T) {
	if _, err := newOuterRingConfig("no-es-un-cidr", ""); err == nil {
		t.Fatal("err = nil, want error (CIDR inválido)")
	}
}

// --- runOuterRingLoop ---

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// runOuterRingLoop NUNCA escanea "al arrancar" (a diferencia de
// scheduler.Scheduler): es justo el punto de haberlo separado (ver su
// comentario) — un primer escaneo inmediato perdería casi siempre la
// carrera contra la autodetección del anillo exterior. Esta prueba
// confirma que, durante una ventana más corta que el intervalo, scan
// nunca se llama.
func TestRunOuterRingLoop_NuncaEscaneaAlArrancar(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	scan := func(context.Context) (*core.ScanResult, error) {
		calls.Add(1)
		return &core.ScanResult{}, nil
	}
	record := func(context.Context, *core.ScanResult, time.Time) {}

	done := make(chan struct{})
	go func() { runOuterRingLoop(ctx, time.Hour, scan, record, discardLogger()); close(done) }()

	time.Sleep(50 * time.Millisecond)
	if n := calls.Load(); n != 0 {
		t.Fatalf("scan se llamó %d veces antes de un solo tick, want 0", n)
	}
	cancel()
	<-done
}

// Con un intervalo corto, sí escanea en cada tick y pasa el resultado a
// record.
func TestRunOuterRingLoop_EscaneaCadaTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var scans, records atomic.Int32
	scan := func(context.Context) (*core.ScanResult, error) {
		scans.Add(1)
		return &core.ScanResult{Gateway: "192.168.100.1"}, nil
	}
	record := func(_ context.Context, res *core.ScanResult, _ time.Time) {
		if res == nil || res.Gateway != "192.168.100.1" {
			t.Errorf("record recibió %+v, want el ScanResult de scan", res)
		}
		records.Add(1)
	}

	done := make(chan struct{})
	go func() { runOuterRingLoop(ctx, 10*time.Millisecond, scan, record, discardLogger()); close(done) }()

	time.Sleep(55 * time.Millisecond) // ~5 ticks
	cancel()
	<-done

	if n := scans.Load(); n < 2 {
		t.Fatalf("scans = %d, want al menos 2 en ~5 intervalos", n)
	}
	if scans.Load() != records.Load() {
		t.Fatalf("scans=%d records=%d, want iguales (cada scan exitoso llama a record)", scans.Load(), records.Load())
	}
}

// Un error de scan (que no sea por el ctx cancelado) no debe tumbar el
// loop: el siguiente tick lo intenta de nuevo.
func TestRunOuterRingLoop_ErrorDeScanNoDetieneElLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls, records atomic.Int32
	scan := func(context.Context) (*core.ScanResult, error) {
		calls.Add(1)
		return nil, errBoom
	}
	record := func(context.Context, *core.ScanResult, time.Time) { records.Add(1) }

	done := make(chan struct{})
	go func() { runOuterRingLoop(ctx, 10*time.Millisecond, scan, record, discardLogger()); close(done) }()

	time.Sleep(55 * time.Millisecond)
	cancel()
	<-done

	if n := calls.Load(); n < 2 {
		t.Fatalf("calls = %d, want al menos 2 (el error no detuvo el loop)", n)
	}
	if n := records.Load(); n != 0 {
		t.Fatalf("records = %d, want 0 (nunca se llama con un scan fallido)", n)
	}
}

type boomErr string

func (e boomErr) Error() string { return string(e) }

const errBoom = boomErr("boom")
