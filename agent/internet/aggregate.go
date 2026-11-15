package internet

import "time"

// Window acumula Sample dentro de una misma ventana de tiempo (un minuto o
// una hora, según `bucket`) y la cierra apenas llega una muestra de la
// ventana siguiente — puro, sin ticker propio: quien llama (el medidor de
// 30 s en cmd/vigia-agent) decide cuándo alimentarla, y es el `At` de cada
// Sample el que decide a qué ventana pertenece (time.Time.Truncate(bucket),
// en UTC). Dos Window independientes (una de time.Minute, otra de
// time.Hour) alimentadas con el mismo Sample dan, respectivamente, las
// filas de "por minuto (7 días)" y "por hora (90 días)" del contrato (ver
// docs/agente-api.md).
type Window struct {
	bucket time.Duration

	// current: inicio de la ventana abierta. Zero value (time.Time{}) =
	// todavía no llegó ninguna muestra.
	current time.Time

	router, modem, inet, dns accumulator
}

// NewWindow crea una Window vacía para el tamaño de bucket dado
// (típicamente time.Minute o time.Hour).
func NewWindow(bucket time.Duration) *Window {
	return &Window{bucket: bucket}
}

// Feed agrega s a la ventana abierta. Si s cae en una ventana distinta de
// la abierta (incluida la primera llamada), cierra la anterior —si tenía
// algo— y empieza una nueva con s ya adentro. ok es false en la primera
// llamada (no hay ventana anterior que cerrar) y true cada vez que una
// ventana se cierra de verdad.
func (w *Window) Feed(s Sample) (closed Agg, ok bool) {
	bucketStart := s.At.UTC().Truncate(w.bucket)

	if w.current.IsZero() {
		w.current = bucketStart
		w.add(s)
		return Agg{}, false
	}

	if bucketStart.Equal(w.current) {
		w.add(s)
		return Agg{}, false
	}

	closed = w.snapshot()
	w.reset(bucketStart)
	w.add(s)
	return closed, true
}

// Flush cierra la ventana abierta (si tiene alguna muestra) sin esperar a
// que llegue una de la ventana siguiente — para no perder la última
// ventana parcial al apagar el agente (ver cmd/vigia-agent, apagado
// limpio).
func (w *Window) Flush() (Agg, bool) {
	if w.current.IsZero() {
		return Agg{}, false
	}
	closed := w.snapshot()
	w.current = time.Time{}
	w.router, w.modem, w.inet, w.dns = accumulator{}, accumulator{}, accumulator{}, accumulator{}
	return closed, true
}

func (w *Window) add(s Sample) {
	w.router.add(s.Router)
	w.modem.add(s.Modem)
	w.inet.add(&s.Internet)
	w.dns.add(&s.DNS)
}

func (w *Window) snapshot() Agg {
	return Agg{
		At:       w.current,
		Router:   w.router.stats(),
		Modem:    w.modem.stats(),
		Internet: w.inet.stats(),
		DNS:      w.dns.stats(),
	}
}

func (w *Window) reset(bucketStart time.Time) {
	w.current = bucketStart
	w.router, w.modem, w.inet, w.dns = accumulator{}, accumulator{}, accumulator{}, accumulator{}
}

// accumulator es el estado mutable de UN tramo dentro de una Window en
// construcción (ver SegmentStats, su versión ya cerrada/inmutable).
type accumulator struct {
	sent, lost          int
	minMs, maxMs, sumMs float64
	n                   int // mediciones que SÍ contestaron: para el promedio.
}

// add agrega una medición. seg == nil significa "no se intentó este tramo
// en este ciclo" (ver Sample.Router/Modem) y no cuenta ni como enviada ni
// como perdida.
func (a *accumulator) add(seg *Segment) {
	if seg == nil {
		return
	}
	a.sent++
	if !seg.OK {
		a.lost++
		return
	}
	a.n++
	if a.n == 1 || seg.MS < a.minMs {
		a.minMs = seg.MS
	}
	if a.n == 1 || seg.MS > a.maxMs {
		a.maxMs = seg.MS
	}
	a.sumMs += seg.MS
}

func (a accumulator) stats() SegmentStats {
	st := SegmentStats{Sent: a.sent, Lost: a.lost}
	if a.n > 0 {
		st.MinMs, st.MaxMs, st.AvgMs = a.minMs, a.maxMs, a.sumMs/float64(a.n)
	}
	return st
}
