package internet

import "testing"

func TestEvaluateState_DownManda(t *testing.T) {
	if got := EvaluateState(true, 0, 10, 10); got != StateDown {
		t.Fatalf("got %v, want down", got)
	}
}

func TestEvaluateState_DegradadoPorPerdida(t *testing.T) {
	if got := EvaluateState(false, 5.1, 10, 10); got != StateDegraded {
		t.Fatalf("got %v, want degraded (pérdida > 5%%)", got)
	}
	if got := EvaluateState(false, 5, 10, 10); got != StateUp {
		t.Fatalf("got %v, want up (5%% exacto no es > 5)", got)
	}
}

func TestEvaluateState_DegradadoPorLatencia(t *testing.T) {
	if got := EvaluateState(false, 0, 31, 10); got != StateDegraded {
		t.Fatalf("got %v, want degraded (31 > 3×10)", got)
	}
	if got := EvaluateState(false, 0, 30, 10); got != StateUp {
		t.Fatalf("got %v, want up (30 = 3×10, no mayor)", got)
	}
}

func TestEvaluateState_SinMedianaTodaviaNuncaDegradaPorLatencia(t *testing.T) {
	if got := EvaluateState(false, 0, 999, 0); got != StateUp {
		t.Fatalf("got %v, want up (sin historia, no se puede comparar)", got)
	}
}

func TestEvaluateState_Up(t *testing.T) {
	if got := EvaluateState(false, 0, 10, 10); got != StateUp {
		t.Fatalf("got %v, want up", got)
	}
}

func TestClassifyIPv6(t *testing.T) {
	cases := []struct {
		hasGlobal, dialOK bool
		want              IPv6Status
	}{
		{false, false, IPv6None},
		{false, true, IPv6None}, // sin dirección global, ni se intenta: dialOK se ignora.
		{true, true, IPv6OK},
		{true, false, IPv6Broken},
	}
	for _, c := range cases {
		if got := ClassifyIPv6(c.hasGlobal, c.dialOK); got != c.want {
			t.Fatalf("ClassifyIPv6(%v, %v) = %v, want %v", c.hasGlobal, c.dialOK, got, c.want)
		}
	}
}

func TestMedian(t *testing.T) {
	if got := Median(nil); got != 0 {
		t.Fatalf("Median(nil) = %v, want 0", got)
	}
	if got := Median([]float64{5}); got != 5 {
		t.Fatalf("Median([5]) = %v, want 5", got)
	}
	if got := Median([]float64{1, 3, 2}); got != 2 {
		t.Fatalf("Median([1,3,2]) = %v, want 2", got)
	}
	if got := Median([]float64{1, 2, 3, 4}); got != 2.5 {
		t.Fatalf("Median([1,2,3,4]) = %v, want 2.5", got)
	}
}

func TestSettings_Valid(t *testing.T) {
	if !DefaultSettings().Valid() {
		t.Fatal("DefaultSettings() debe ser válido")
	}
	if (Settings{SpeedTestsPerDay: -1}).Valid() {
		t.Fatal("-1 no debe ser válido")
	}
	if (Settings{SpeedTestsPerDay: 7}).Valid() {
		t.Fatal("7 no debe ser válido (tope 6)")
	}
	if !(Settings{SpeedTestsPerDay: 0}).Valid() {
		t.Fatal("0 debe ser válido")
	}
	if !(Settings{SpeedTestsPerDay: 6}).Valid() {
		t.Fatal("6 debe ser válido")
	}
}
