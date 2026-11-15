package wake

import (
	"testing"
	"time"
)

func TestDetector_PrimeraLlamadaNuncaSospecha(t *testing.T) {
	d := NewDetector(5*time.Minute, 2.5)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	asleep, gap := d.Feed(base)
	if asleep || gap != 0 {
		t.Fatalf("primera llamada: asleep=%v gap=%v, want false/0", asleep, gap)
	}
}

func TestDetector_HuecoNormalNoSospecha(t *testing.T) {
	d := NewDetector(5*time.Minute, 2.5)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	d.Feed(base)
	// El ciclo tardó justo lo esperado (5 min): ni cerca del umbral (12.5 min).
	asleep, gap := d.Feed(base.Add(5 * time.Minute))
	if asleep {
		t.Fatalf("hueco de 5 min (= expected) no debería sospechar suspensión, gap=%v", gap)
	}
}

func TestDetector_JitterPorDebajoDelUmbralNoSospecha(t *testing.T) {
	d := NewDetector(5*time.Minute, 2.5)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	d.Feed(base)
	// Un escaneo que tardó el doble de lo normal (10 min, < 12.5 de umbral):
	// tardío, pero no sospechoso de suspensión.
	if asleep, _ := d.Feed(base.Add(10 * time.Minute)); asleep {
		t.Fatal("10 min (< umbral de 12.5 min) no debería sospechar suspensión")
	}
}

func TestDetector_HuecoGrandeSospechaSuspension(t *testing.T) {
	d := NewDetector(5*time.Minute, 2.5)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	d.Feed(base)
	asleep, gap := d.Feed(base.Add(2 * time.Hour))
	if !asleep {
		t.Fatal("hueco de 2h (>> umbral de 12.5 min) debería sospechar suspensión")
	}
	if gap != 2*time.Hour {
		t.Fatalf("gap = %v, want 2h", gap)
	}
}

func TestDetector_TrasSospecharVuelveANoSospecharConHuecosNormales(t *testing.T) {
	d := NewDetector(30*time.Second, 2.5)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	d.Feed(base)
	if asleep, _ := d.Feed(base.Add(time.Hour)); !asleep {
		t.Fatal("1h sobre un ciclo de 30s debería sospechar suspensión")
	}
	// El siguiente ciclo, ya "despierto", vuelve a andar normal: no debe
	// seguir sospechando para siempre por el hueco ya pasado.
	if asleep, _ := d.Feed(base.Add(time.Hour).Add(30 * time.Second)); asleep {
		t.Fatal("tras el hueco, un ciclo normal no debería seguir sospechando")
	}
}

func TestDetector_RelojQueRetrocedeNoSospecha(t *testing.T) {
	d := NewDetector(5*time.Minute, 2.5)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	d.Feed(base)
	if asleep, gap := d.Feed(base.Add(-time.Hour)); asleep || gap >= 0 {
		t.Fatalf("reloj retrocedido: asleep=%v gap=%v, want false/negativo", asleep, gap)
	}
}

func TestDetector_FactorPorDefecto(t *testing.T) {
	d := NewDetector(time.Minute, 0)
	if want := 150 * time.Second; d.Threshold() != want {
		t.Fatalf("Threshold() = %v, want %v (factor por defecto 2.5)", d.Threshold(), want)
	}
}
