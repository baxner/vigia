package wake

import (
	"testing"
	"time"
)

func TestDetectorStripsMonotonicClock(t *testing.T) {
	// Simula que el sistema suspendió: el reloj de pared avanzó 10 minutos, pero
	// el reloj monotónico adjunto a time.Time (como tras sleep en macOS) no
	// habría avanzado. Al aplicar Round(0), Feed debe medir los 10 minutos reales.
	d := NewDetector(2*time.Minute, 0)
	t1 := time.Now()
	if asleep, _ := d.Feed(t1); asleep {
		t.Fatal("primera llamada no debe reportar suspensión")
	}

	// Construye un t2 con +10m de wall clock pero solo +1s monotónico (t1.Add(time.Second)
	// lleva el reloj monotónico de t1 + 1s; si re-armamos con pared +10m manteniendo
	// monotonic, Sub sin Round(0) devolvería 1s, pero en Go solo podemos observar
	// cómo Round(0) preserva el delta de pared cuando hay monotonic vs no).
	// Más directo: t1 lleva reloj monotónico de Go; al sumar pared usando time.Date
	// (sin monotónico) y compararlo con otro instant, verifica que d.last no retiene
	// lectura monotónica.
	t2 := t1.Add(10 * time.Minute)
	asleep, gap := d.Feed(t2)
	if !asleep || gap != 10*time.Minute {
		t.Fatalf("got (asleep=%v, gap=%v), want (true, 10m)", asleep, gap)
	}
	if d.last.Round(0) != d.last {
		t.Fatalf("d.last conserva lectura monotónica")
	}
}
