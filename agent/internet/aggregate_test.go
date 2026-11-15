package internet

import (
	"testing"
	"time"
)

func minuteAt(min, sec int) time.Time {
	return time.Date(2026, 10, 11, 12, min, sec, 0, time.UTC)
}

func TestWindow_PrimeraMuestraNoCierraNada(t *testing.T) {
	w := NewWindow(time.Minute)
	_, closed := w.Feed(Sample{At: minuteAt(0, 0), Router: okp(1), Modem: okp(1), Internet: ok(10), DNS: ok(5)})
	if closed {
		t.Fatal("la primera muestra no debe cerrar ninguna ventana")
	}
}

func TestWindow_CierraAlCruzarElMinuto(t *testing.T) {
	w := NewWindow(time.Minute)
	w.Feed(Sample{At: minuteAt(0, 0), Router: okp(2), Modem: okp(4), Internet: ok(10), DNS: ok(5)})
	w.Feed(Sample{At: minuteAt(0, 30), Router: okp(4), Modem: failp(), Internet: fail(), DNS: ok(7)})

	agg, closed := w.Feed(Sample{At: minuteAt(1, 0), Router: okp(1), Modem: okp(1), Internet: ok(1), DNS: ok(1)})
	if !closed {
		t.Fatal("la 3ª muestra (minuto siguiente) debe cerrar el minuto 0")
	}
	if !agg.At.Equal(minuteAt(0, 0)) {
		t.Fatalf("At = %v, want %v", agg.At, minuteAt(0, 0))
	}
	if agg.Router.Sent != 2 || agg.Router.Lost != 0 {
		t.Fatalf("Router = %+v, want Sent=2 Lost=0", agg.Router)
	}
	if agg.Router.AvgMs != 3 { // (2+4)/2
		t.Fatalf("Router.AvgMs = %v, want 3", agg.Router.AvgMs)
	}
	if agg.Modem.Sent != 2 || agg.Modem.Lost != 1 {
		t.Fatalf("Modem = %+v, want Sent=2 Lost=1", agg.Modem)
	}
	if agg.Modem.AvgMs != 4 { // solo la que contestó
		t.Fatalf("Modem.AvgMs = %v, want 4", agg.Modem.AvgMs)
	}
	if agg.Internet.Sent != 2 || agg.Internet.Lost != 1 {
		t.Fatalf("Internet = %+v, want Sent=2 Lost=1", agg.Internet)
	}
	if agg.Internet.LossPct() != 50 {
		t.Fatalf("Internet.LossPct() = %v, want 50", agg.Internet.LossPct())
	}
	if agg.DNS.Sent != 2 || agg.DNS.AvgMs != 6 { // (5+7)/2
		t.Fatalf("DNS = %+v, want Sent=2 AvgMs=6", agg.DNS)
	}
}

func TestWindow_ModemNuncaIntentadoQuedaSinDato(t *testing.T) {
	w := NewWindow(time.Minute)
	w.Feed(Sample{At: minuteAt(0, 0), Router: okp(2), Modem: nil, Internet: ok(10), DNS: ok(5)})
	agg, closed := w.Feed(Sample{At: minuteAt(1, 0), Router: okp(2), Modem: nil, Internet: ok(10), DNS: ok(5)})
	if !closed {
		t.Fatal("debió cerrar el minuto 0")
	}
	if agg.Modem.Sent != 0 {
		t.Fatalf("Modem.Sent = %v, want 0 (nunca se intentó)", agg.Modem.Sent)
	}
	if agg.Modem.HasLatency() {
		t.Fatal("Modem.HasLatency() debe ser false sin datos")
	}
	if agg.Modem.LossPct() != 0 {
		t.Fatalf("Modem.LossPct() = %v, want 0 (no se intentó, no es pérdida)", agg.Modem.LossPct())
	}
}

func TestSegmentStats_TodoFallidoSinLatenciaYPerdidaTotal(t *testing.T) {
	st := SegmentStats{Sent: 2, Lost: 2}
	if st.HasLatency() {
		t.Fatal("HasLatency() debe ser false cuando todo falló")
	}
	if st.LossPct() != 100 {
		t.Fatalf("LossPct() = %v, want 100", st.LossPct())
	}
}

func TestWindow_Flush_CierraLaVentanaAbierta(t *testing.T) {
	w := NewWindow(time.Minute)
	w.Feed(Sample{At: minuteAt(0, 0), Router: okp(2), Modem: okp(2), Internet: ok(10), DNS: ok(5)})
	agg, closed := w.Flush()
	if !closed {
		t.Fatal("Flush debe cerrar la ventana con datos")
	}
	if agg.Router.Sent != 1 {
		t.Fatalf("Router.Sent = %v, want 1", agg.Router.Sent)
	}
	if _, closedAgain := w.Flush(); closedAgain {
		t.Fatal("un segundo Flush sin muestras nuevas no debe cerrar nada")
	}
}

func TestWindow_HoraAgrupaVariosMinutos(t *testing.T) {
	w := NewWindow(time.Hour)
	hourStart := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	w.Feed(Sample{At: hourStart, Router: okp(2), Modem: okp(2), Internet: ok(10), DNS: ok(5)})
	w.Feed(Sample{At: hourStart.Add(30 * time.Minute), Router: okp(4), Modem: okp(2), Internet: ok(20), DNS: ok(5)})

	agg, closed := w.Feed(Sample{At: hourStart.Add(time.Hour), Router: okp(1), Modem: okp(1), Internet: ok(1), DNS: ok(1)})
	if !closed {
		t.Fatal("debió cerrar la hora anterior")
	}
	if !agg.At.Equal(hourStart) {
		t.Fatalf("At = %v, want %v", agg.At, hourStart)
	}
	if agg.Router.Sent != 2 || agg.Router.AvgMs != 3 {
		t.Fatalf("Router = %+v, want Sent=2 AvgMs=3", agg.Router)
	}
}
