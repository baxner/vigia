package internet

import "time"

// OutageDetector implementa "Caídas" (ver docs/agente-api.md): el tramo
// internet falla en 2 mediciones seguidas abre la caída, 2 seguidas que
// contestan la cierran. Puro y sin reloj propio (el `At` de cada Sample es
// quien manda); un OutageDetector vive mientras dure el proceso, igual que
// agent/store.Store.AgentHealthState sobrevive reinicios para AGENT_BLIND
// —a diferencia de ese caso, acá el estado (consecFail/consecOK) SÍ se
// pierde al reiniciar el agente: una caída que ya estaba abierta antes de
// un reinicio queda sin cerrar hasta que el medidor junte 2 fallas o 2
// aciertos de nuevo (ver cmd/vigia-agent/internet.go, que resume la fila
// abierta de agent/store al arrancar, pero no los contadores).
//
// Start/End de una Outage son la PRIMERA muestra de cada racha, no la que
// la CONFIRMA (la 2ª): así la duración reportada es la caída real, no la
// caída más el intervalo de una muestra de más en cada punta.
type OutageDetector struct {
	consecFail int
	consecOK   int

	firstFailAt     time.Time
	firstFailSample Sample
	firstOKAt       time.Time

	open *Outage
}

// NewOutageDetector crea un detector sin caída abierta.
func NewOutageDetector() *OutageDetector {
	return &OutageDetector{}
}

// ResumeOutageDetector crea un detector con una caída YA abierta (ver
// agent/store.Store.OpenOutage): para cuando el agente se reinicia a
// mitad de una caída y GET /api/v1/internet no debe "olvidarla". Los
// contadores consecFail/consecOK quedan en cero: el medidor necesita
// juntar 2 fallas o 2 aciertos de verdad, DESPUÉS de reiniciar, para abrir
// otra caída o cerrar esta (ver el comentario del tipo).
func ResumeOutageDetector(open Outage) *OutageDetector {
	o := open
	return &OutageDetector{open: &o}
}

// Open: true si hay una caída abierta ahora mismo.
func (d *OutageDetector) Open() bool { return d.open != nil }

// Current devuelve la caída abierta (y true), o Outage{}, false si no hay
// ninguna.
func (d *OutageDetector) Current() (Outage, bool) {
	if d.open == nil {
		return Outage{}, false
	}
	return *d.open, true
}

// Reset olvida las rachas consecFail/consecOK a medio contar, SIN tocar
// una caída que ya estuviera abierta (open se conserva igual que antes:
// ver el comentario del tipo sobre qué sobrevive a un reinicio del
// proceso). Para cuando quien llama detecta un hueco grande entre
// muestras (ver agent/wake y cmd/vigia-agent/internet.go, "Suspensión"):
// sin este Reset, una falla de ANTES del hueco (ya con consecFail == 1,
// todavía sin confirmar) se combinaría con la primera muestra de DESPUÉS
// como si fueran dos fallas seguidas de verdad, abriendo una caída
// fantasma con Start en la falla vieja — como si hubiera durado toda la
// suspensión. Tras Reset, hacen falta 2 muestras de verdad, ya
// despierto, para abrir o cerrar algo.
func (d *OutageDetector) Reset() {
	d.consecFail = 0
	d.consecOK = 0
	d.firstFailAt = time.Time{}
	d.firstFailSample = Sample{}
	d.firstOKAt = time.Time{}
}

// Feed procesa una muestra y devuelve la Outage que se acaba de ABRIR o
// CERRAR en este instante (con End ya puesto si se cerró), o nil si esta
// muestra no cambió el estado (el caso normal: ni la 2ª falla ni el 2º
// acierto todavía, o una racha de 1 que no llegó a confirmarse).
func (d *OutageDetector) Feed(s Sample) *Outage {
	if s.Internet.OK {
		return d.feedOK(s)
	}
	return d.feedFail(s)
}

func (d *OutageDetector) feedOK(s Sample) *Outage {
	if d.consecOK == 0 {
		d.firstOKAt = s.At
	}
	d.consecOK++
	d.consecFail = 0

	if d.open == nil || d.consecOK < 2 {
		return nil
	}
	end := d.firstOKAt
	closed := *d.open
	closed.End = &end
	d.open = nil
	return &closed
}

func (d *OutageDetector) feedFail(s Sample) *Outage {
	if d.consecFail == 0 {
		d.firstFailAt = s.At
		d.firstFailSample = s
	}
	d.consecFail++
	d.consecOK = 0

	if d.open != nil || d.consecFail < 2 {
		return nil
	}
	o := Outage{Start: d.firstFailAt, Blame: blameFor(d.firstFailSample)}
	d.open = &o
	opened := o
	return &opened
}

// blameFor decide el culpable de una caída a partir de la PRIMERA muestra
// que falló (ver docs/agente-api.md, "Culpable"): Router == nil ("no se
// intentó medir el router") es lo único que da BlameUnknown — cualquier
// otra combinación distingue lan/modem/isp sin ambigüedad.
func blameFor(s Sample) Blame {
	if s.Router == nil {
		return BlameUnknown
	}
	if !s.Router.OK {
		return BlameLAN
	}
	if s.Modem != nil && !s.Modem.OK {
		return BlameModem
	}
	// Router contesta; módem contesta o no se conoce (anillo exterior
	// todavía no autodetectado): no hay más tramos de la casa que revisar,
	// así que el problema está más allá — el proveedor.
	return BlameISP
}
