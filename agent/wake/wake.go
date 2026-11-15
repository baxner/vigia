// Package wake detecta, de forma portable y sin E/S, cuando el hueco entre
// dos instantes sucesivos de un ciclo periódico (un escaneo, una muestra de
// internet...) es tan grande que lo más probable es que la máquina haya
// estado suspendida (dormida) en el medio, y no que el ciclo simplemente
// haya tardado un poco más de lo normal.
//
// Por qué por el hueco entre muestras y no comparando el reloj de pared
// contra uno monotónico: ese segundo método depende de si el monotónico de
// la plataforma sigue avanzando durante la suspensión o no (varía entre
// sistemas, y Go no expone cuál usa en cada uno) — el hueco entre dos
// time.Time que YA pasó quien llama es portable de verdad: no importa qué
// reloj los generó, cuánto duró la suspensión ni si el proceso estuvo
// congelado o no mientras tanto. Detector no lee ningún reloj — Feed
// recibe el `now` de quien llama — así se prueba con un reloj inyectado,
// sin dormir la máquina de verdad (ver wake_test.go).
package wake

import "time"

// Detector decide, muestra a muestra, si el hueco contra la anterior es
// sospechoso de suspensión. No concurrente por sí mismo: cada sitio que
// lo usa (el escaneo de 5 min, el medidor de internet de 30 s) tiene su
// propia instancia y la alimenta siempre desde el mismo lugar de un solo
// hilo (ver cmd/vigia-agent/run.go e internet.go).
type Detector struct {
	expected time.Duration
	factor   float64
	last     time.Time
	haveLast bool
}

// NewDetector crea un Detector para un ciclo cuyo período normal es
// expected. factor es cuántas veces expected tiene que pasar entre dos
// muestras para sospechar suspensión (ver Feed); factor <= 0 usa 2.5 (un
// punto medio: muy por encima del jitter normal de un ticker, muy por
// debajo de lo que tardaría una suspensión real de minutos u horas).
func NewDetector(expected time.Duration, factor float64) *Detector {
	if factor <= 0 {
		factor = 2.5
	}
	return &Detector{expected: expected, factor: factor}
}

// Threshold: el hueco a partir del cual Feed sospecha suspensión
// (expected × factor). Expuesto para que quien llama pueda explicar en
// sus logs/pruebas de qué número habla.
func (d *Detector) Threshold() time.Duration {
	return time.Duration(float64(d.expected) * d.factor)
}

// Feed registra `now` y compara el hueco contra la última llamada. asleep
// es true si ese hueco superó el umbral (ver Threshold) — en ese caso,
// quien llama debe tratar el ciclo actual como "recién despierto": no
// acumular ese hueco como una caída/desconexión real, y typically disparar
// de inmediato lo que haya estado esperando el próximo tick. La primera
// llamada nunca es "asleep" (no hay hueco todavía que medir) — solo fija
// la base para la siguiente.
func (d *Detector) Feed(now time.Time) (asleep bool, gap time.Duration) {
	now = now.Round(0)
	if !d.haveLast {
		d.last = now
		d.haveLast = true
		return false, 0
	}
	gap = now.Sub(d.last)
	d.last = now
	if gap < 0 {
		// El reloj de pared retrocedió (ajuste de hora, NTP...): no es el
		// caso que este Detector cubre, pero tampoco hay que confundirlo
		// con una suspensión — de haberla, el hueco es positivo y grande,
		// nunca negativo.
		return false, gap
	}
	return gap > d.Threshold(), gap
}
