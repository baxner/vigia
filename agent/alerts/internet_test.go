package alerts

import (
	"testing"
	"time"

	"github.com/baxner/vigia/agent/internet"
)

func TestEvaluateInternetDown(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	a := EvaluateInternetDown(internet.BlameISP, testNetwork, start, DefaultSettings())
	if a == nil {
		t.Fatal("EvaluateInternetDown = nil, want alerta")
	}
	if a.Type != InternetDown || a.Severity != High {
		t.Fatalf("type/severity = %s/%s, want INTERNET_DOWN/high", a.Type, a.Severity)
	}
	if a.Title != "Se cayó el internet" {
		t.Fatalf("title = %q", a.Title)
	}
	if a.Body == "" {
		t.Fatal("body vacío")
	}
	if a.DeviceID != nil {
		t.Fatalf("deviceId = %v, want nil", a.DeviceID)
	}
	if a.Network != testNetwork {
		t.Fatalf("network = %q, want %q", a.Network, testNetwork)
	}
	if !a.At.Equal(start) {
		t.Fatalf("at = %v, want %v", a.At, start)
	}

	// Apagado en settings: nil.
	off := DefaultSettings()
	off.InternetDown = false
	if a := EvaluateInternetDown(internet.BlameISP, testNetwork, start, off); a != nil {
		t.Fatalf("con InternetDown=false, want nil, got %+v", a)
	}
}

func TestEvaluateInternetDown_TextoPorCulpable(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		blame internet.Blame
		want  string
	}{
		{internet.BlameLAN, "Se cayó el internet · El problema parece estar en tu router: no está respondiendo."},
		{internet.BlameModem, "Se cayó el internet · El problema parece estar en el módem (tu router responde, pero el módem no)."},
		{internet.BlameISP, "Se cayó el internet · El problema parece estar en tu proveedor (tu router y el módem responden)."},
		{internet.BlameUnknown, "Se cayó el internet · No se pudo determinar en qué parte de la red está el problema."},
	}
	for _, c := range cases {
		a := EvaluateInternetDown(c.blame, testNetwork, start, DefaultSettings())
		if a.Body != c.want {
			t.Fatalf("blame=%v body = %q, want %q", c.blame, a.Body, c.want)
		}
	}
}

func TestEvaluateInternetRestored(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Minute)

	a := EvaluateInternetRestored(internet.BlameISP, testNetwork, start, end, DefaultSettings())
	if a == nil {
		t.Fatal("EvaluateInternetRestored = nil, want alerta")
	}
	if a.Type != InternetRestored || a.Severity != Low {
		t.Fatalf("type/severity = %s/%s, want INTERNET_RESTORED/low", a.Type, a.Severity)
	}
	if a.Title != "Volvió el internet" {
		t.Fatalf("title = %q", a.Title)
	}
	want := "Volvió el internet · Estuvo caído 3 min. Culpable: tu proveedor."
	if a.Body != want {
		t.Fatalf("body = %q, want %q", a.Body, want)
	}
	if !a.At.Equal(end) {
		t.Fatalf("at = %v, want %v (el fin de la caída)", a.At, end)
	}

	off := DefaultSettings()
	off.InternetRestored = false
	if a := EvaluateInternetRestored(internet.BlameISP, testNetwork, start, end, off); a != nil {
		t.Fatalf("con InternetRestored=false, want nil, got %+v", a)
	}
}

func TestEvaluateInternetDownRestored_DedupKeyPorInicioDeLaCaida(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)

	down := EvaluateInternetDown(internet.BlameISP, testNetwork, start, DefaultSettings())
	restored := EvaluateInternetRestored(internet.BlameISP, testNetwork, start, end, DefaultSettings())

	if down.DedupKey == restored.DedupKey {
		t.Fatal("DOWN y RESTORED no deben compartir DedupKey (son tipos distintos)")
	}

	// Reevaluar la MISMA caída (p. ej. tras un reinicio a mitad de un
	// ciclo) debe dar la MISMA DedupKey — es la restricción UNIQUE de
	// agent/store la que evita que se duplique el aviso.
	downAgain := EvaluateInternetDown(internet.BlameISP, testNetwork, start, DefaultSettings())
	if down.DedupKey != downAgain.DedupKey {
		t.Fatalf("DedupKey cambió al reevaluar la misma caída: %q vs %q", down.DedupKey, downAgain.DedupKey)
	}
	restoredAgain := EvaluateInternetRestored(internet.BlameISP, testNetwork, start, end, DefaultSettings())
	if restored.DedupKey != restoredAgain.DedupKey {
		t.Fatalf("DedupKey cambió al reevaluar la misma recuperación: %q vs %q", restored.DedupKey, restoredAgain.DedupKey)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Second, "45 s"},
		{3 * time.Minute, "3 min"},
		{90 * time.Second, "2 min"}, // redondea al minuto más cercano
		{time.Hour, "1 h"},
		{time.Hour + 12*time.Minute, "1 h 12 min"},
	}
	for _, c := range cases {
		if got := formatDuration(c.d); got != c.want {
			t.Fatalf("formatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestEnabled_InternetDownRestored(t *testing.T) {
	s := DefaultSettings()
	if !s.Enabled(InternetDown) || !s.Enabled(InternetRestored) {
		t.Fatal("deben venir encendidas por defecto")
	}
}
