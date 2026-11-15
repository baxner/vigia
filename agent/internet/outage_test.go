package internet

import (
	"testing"
	"time"
)

func at(sec int) time.Time {
	return time.Date(2026, 10, 11, 12, 0, sec, 0, time.UTC)
}

func ok(ms float64) Segment   { return Segment{MS: ms, OK: true} }
func fail() Segment           { return Segment{OK: false} }
func okp(ms float64) *Segment { s := ok(ms); return &s }
func failp() *Segment         { s := fail(); return &s }

// sample construye una muestra con router/módem OK (los valores típicos de
// "todo bien") e internet según internetOK, en el instante at(sec).
func sample(sec int, internetOK bool) Sample {
	seg := fail()
	if internetOK {
		seg = ok(20)
	}
	return Sample{At: at(sec), Router: okp(2), Modem: okp(4), Internet: seg, DNS: ok(10)}
}

func TestOutageDetector_CaidaCortaNoCuenta(t *testing.T) {
	d := NewOutageDetector()
	// Una sola falla...
	if ev := d.Feed(sample(0, false)); ev != nil {
		t.Fatalf("1 falla no debe abrir nada, got %+v", ev)
	}
	if d.Open() {
		t.Fatal("no debería haber caída abierta tras 1 falla")
	}
	// ...seguida de un acierto: nunca llegó a confirmarse, no cuenta.
	if ev := d.Feed(sample(30, true)); ev != nil {
		t.Fatalf("recuperarse tras 1 sola falla no debe generar evento, got %+v", ev)
	}
	if d.Open() {
		t.Fatal("no debería haber caída abierta")
	}
}

func TestOutageDetector_AbreConLaPrimeraFallaYCulpaAlISP(t *testing.T) {
	d := NewOutageDetector()
	if ev := d.Feed(sample(0, false)); ev != nil {
		t.Fatalf("1ª falla no debe abrir todavía, got %+v", ev)
	}
	ev := d.Feed(sample(30, false))
	if ev == nil {
		t.Fatal("la 2ª falla seguida debe abrir la caída")
	}
	if !ev.Start.Equal(at(0)) {
		t.Fatalf("Start = %v, want %v (la PRIMERA falla, no la que confirma)", ev.Start, at(0))
	}
	if ev.End != nil {
		t.Fatalf("End = %v, want nil (sigue abierta)", ev.End)
	}
	if ev.Blame != BlameISP {
		t.Fatalf("Blame = %v, want isp (router y módem contestan)", ev.Blame)
	}
	if !d.Open() {
		t.Fatal("debería quedar abierta")
	}
}

func TestOutageDetector_CulpaAlRouter(t *testing.T) {
	d := NewOutageDetector()
	fallando := Sample{At: at(0), Router: failp(), Modem: okp(4), Internet: fail(), DNS: fail()}
	fallando2 := Sample{At: at(30), Router: failp(), Modem: okp(4), Internet: fail(), DNS: fail()}
	d.Feed(fallando)
	ev := d.Feed(fallando2)
	if ev == nil {
		t.Fatal("debió abrir la caída")
	}
	if ev.Blame != BlameLAN {
		t.Fatalf("Blame = %v, want lan (el router no contesta)", ev.Blame)
	}
}

func TestOutageDetector_CulpaAlModem(t *testing.T) {
	d := NewOutageDetector()
	s1 := Sample{At: at(0), Router: okp(2), Modem: failp(), Internet: fail(), DNS: fail()}
	s2 := Sample{At: at(30), Router: okp(2), Modem: failp(), Internet: fail(), DNS: fail()}
	d.Feed(s1)
	ev := d.Feed(s2)
	if ev == nil {
		t.Fatal("debió abrir la caída")
	}
	if ev.Blame != BlameModem {
		t.Fatalf("Blame = %v, want modem (router sí, módem no)", ev.Blame)
	}
}

func TestOutageDetector_CulpaAlISP(t *testing.T) {
	d := NewOutageDetector()
	s1 := Sample{At: at(0), Router: okp(2), Modem: okp(4), Internet: fail(), DNS: fail()}
	s2 := Sample{At: at(30), Router: okp(2), Modem: okp(4), Internet: fail(), DNS: fail()}
	d.Feed(s1)
	ev := d.Feed(s2)
	if ev == nil {
		t.Fatal("debió abrir la caída")
	}
	if ev.Blame != BlameISP {
		t.Fatalf("Blame = %v, want isp (router y módem contestan, internet no)", ev.Blame)
	}
}

func TestOutageDetector_CulpaDesconocidaSinMedicionDelRouter(t *testing.T) {
	d := NewOutageDetector()
	s1 := Sample{At: at(0), Router: nil, Modem: nil, Internet: fail(), DNS: fail()}
	s2 := Sample{At: at(30), Router: nil, Modem: nil, Internet: fail(), DNS: fail()}
	d.Feed(s1)
	ev := d.Feed(s2)
	if ev == nil {
		t.Fatal("debió abrir la caída")
	}
	if ev.Blame != BlameUnknown {
		t.Fatalf("Blame = %v, want unknown (no se intentó medir el router)", ev.Blame)
	}
}

func TestOutageDetector_SinModemConocidoNoBloqueaElCulpableISP(t *testing.T) {
	// El anillo exterior todavía no se autodetectó (Modem == nil, no false):
	// no se puede culpar al módem porque no se sabe nada de él, así que cae
	// a isp (mismo criterio que módem OK).
	d := NewOutageDetector()
	s1 := Sample{At: at(0), Router: okp(2), Modem: nil, Internet: fail(), DNS: fail()}
	s2 := Sample{At: at(30), Router: okp(2), Modem: nil, Internet: fail(), DNS: fail()}
	d.Feed(s1)
	ev := d.Feed(s2)
	if ev == nil || ev.Blame != BlameISP {
		t.Fatalf("Blame = %+v, want isp", ev)
	}
}

func TestOutageDetector_RecuperacionConLaPrimeraDeLasDosOK(t *testing.T) {
	d := NewOutageDetector()
	d.Feed(sample(0, false))
	ev := d.Feed(sample(30, false))
	if ev == nil {
		t.Fatal("debió abrir la caída")
	}

	// Una sola recuperación todavía no cierra nada.
	if closedEv := d.Feed(sample(60, true)); closedEv != nil {
		t.Fatalf("1 acierto no debe cerrar todavía, got %+v", closedEv)
	}
	if !d.Open() {
		t.Fatal("debería seguir abierta tras 1 solo acierto")
	}

	closedEv := d.Feed(sample(90, true))
	if closedEv == nil {
		t.Fatal("la 2ª recuperación seguida debe cerrar la caída")
	}
	if closedEv.End == nil {
		t.Fatal("End no debe quedar nil al cerrar")
	}
	if !closedEv.End.Equal(at(60)) {
		t.Fatalf("End = %v, want %v (el PRIMER acierto, no el que confirma)", closedEv.End, at(60))
	}
	dur := closedEv.DurationSec(at(90))
	if dur != 60 {
		t.Fatalf("DurationSec = %v, want 60 (de at(0) a at(60))", dur)
	}
	if d.Open() {
		t.Fatal("no debería quedar abierta tras cerrarse")
	}
}

func TestOutageDetector_UnaFallaSueltaEntreAciertosNoReabre(t *testing.T) {
	d := NewOutageDetector()
	d.Feed(sample(0, true))
	if ev := d.Feed(sample(30, false)); ev != nil {
		t.Fatalf("1 falla aislada no debe abrir nada, got %+v", ev)
	}
	if ev := d.Feed(sample(60, true)); ev != nil {
		t.Fatalf("recuperarse de una falla que nunca se confirmó no genera evento, got %+v", ev)
	}
	if d.Open() {
		t.Fatal("no debería haber caída abierta")
	}
}

func TestOutage_DurationSec_AbiertaUsaNow(t *testing.T) {
	o := Outage{Start: at(0)}
	if got := o.DurationSec(at(45)); got != 45 {
		t.Fatalf("DurationSec = %v, want 45", got)
	}
}

// --- Reset (ver cmd/vigia-agent/internet.go, "Suspensión") ---

// TestOutageDetector_SinReset_HuecoFabricaCaidaFantasma documenta el bug
// que Reset existe para evitar: una falla ANTES de un hueco grande
// (consecFail == 1, sin confirmar) se combina con la primera muestra de
// DESPUÉS como si las dos fueran parte de la misma racha, abriendo una
// caída con Start en la falla vieja — como si hubiera durado todo el
// hueco (p. ej. toda una suspensión de la máquina).
func TestOutageDetector_SinReset_HuecoFabricaCaidaFantasma(t *testing.T) {
	d := NewOutageDetector()
	d.Feed(sample(0, false)) // 1 falla, sin confirmar todavía.

	// Gran hueco (p. ej. la máquina se durmió) y, al volver, OTRA falla
	// suelta y no relacionada.
	huecoGrande := sample(0, false)
	huecoGrande.At = at(0).Add(2 * time.Hour)
	ev := d.Feed(huecoGrande)
	if ev == nil || ev.End != nil {
		t.Fatal("sin Reset: las dos fallas se combinan y abren una caída fantasma")
	}
	if !ev.Start.Equal(at(0)) {
		t.Fatalf("Start = %v, want %v (la falla de ANTES del hueco, no la de después)", ev.Start, at(0))
	}
}

func TestOutageDetector_Reset_HuecoYaNoFabricaCaidaFantasma(t *testing.T) {
	d := NewOutageDetector()
	d.Feed(sample(0, false)) // 1 falla, sin confirmar todavía.

	d.Reset() // quien llama detectó el hueco grande y llama Reset antes de alimentar la siguiente muestra.

	huecoGrande := sample(0, false)
	huecoGrande.At = at(0).Add(2 * time.Hour)
	if ev := d.Feed(huecoGrande); ev != nil {
		t.Fatalf("tras Reset, 1 sola falla no debe abrir nada, got %+v", ev)
	}
	if d.Open() {
		t.Fatal("no debería haber caída abierta: solo 1 falla real tras el Reset")
	}

	// Hacen falta 2 fallas de VERDAD, ya despierto, para abrir una caída.
	otra := huecoGrande
	otra.At = huecoGrande.At.Add(30 * time.Second)
	ev := d.Feed(otra)
	if ev == nil || !ev.Start.Equal(huecoGrande.At) {
		t.Fatalf("ev = %+v, want abrir con Start = %v (la primera falla DESPUÉS del Reset)", ev, huecoGrande.At)
	}
}

func TestOutageDetector_Reset_NoCierraUnaCaidaYaAbierta(t *testing.T) {
	d := NewOutageDetector()
	d.Feed(sample(0, false))
	d.Feed(sample(30, false)) // abre la caída.
	if !d.Open() {
		t.Fatal("debería haber una caída abierta")
	}

	d.Reset()
	if !d.Open() {
		t.Fatal("Reset no debe cerrar una caída que ya estaba abierta de verdad")
	}
	current, ok := d.Current()
	if !ok || !current.Start.Equal(at(0)) {
		t.Fatalf("Current() = %+v, %v, want Start = %v sin cambios", current, ok, at(0))
	}
}
