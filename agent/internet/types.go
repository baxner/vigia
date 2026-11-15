// Package internet implementa "Internet (v1.4)" (ver docs/agente-api.md y
// docs/plan.md, Fase 7): calidad del internet de la casa, caídas con
// culpable y prueba de velocidad. Este archivo y outage.go/aggregate.go/
// status.go son la parte PURA (sin IO ni reloj propio: quien llama pasa
// `at` o el time.Time de cada muestra) — igual que agent/alerts y
// agent/history, para poder probarla con datos en memoria y sin red ni base
// real. La medición de verdad (TCP/ICMP/DNS, el ciclo de 30 s) vive en
// cmd/vigia-agent (ver internet.go ahí); la persistencia, en
// agent/store/internet.go; el contrato HTTP, en agent/api/internet.go.
package internet

import "time"

// Blame identifica a quién se le puede achacar una caída de internet (ver
// docs/agente-api.md, "Caídas" > "Culpable").
type Blame string

const (
	BlameLAN     Blame = "lan"     // el router (anillo interior) no contesta.
	BlameModem   Blame = "modem"   // el router sí, el módem (anillo exterior) no.
	BlameISP     Blame = "isp"     // router y módem sí, internet no.
	BlameUnknown Blame = "unknown" // no se pudo medir el tramo router: no se sabe.
)

// State es el estado agregado de internet AHORA (ver docs/agente-api.md,
// InternetStatus.state).
type State string

const (
	StateUp       State = "up"
	StateDown     State = "down"
	StateDegraded State = "degraded"
)

// IPv6Status es el resultado de la prueba de salida por IPv6 (ver
// docs/agente-api.md: "ok|broken|none").
type IPv6Status string

const (
	IPv6OK     IPv6Status = "ok"
	IPv6Broken IPv6Status = "broken"
	IPv6None   IPv6Status = "none"
)

// Segment es el resultado de UNA medición de UN tramo (router, módem,
// internet o DNS) en un instante. MS solo es significativo cuando OK es
// true (ver Window.add): una medición fallida no tiene un tiempo de
// respuesta real que promediar.
type Segment struct {
	MS float64
	OK bool
}

// Sample es una vuelta completa del medidor de 30 s (ver cmd/vigia-agent,
// "Medidor cada 30 s" en docs/plan.md): un tramo por destino, más DNS.
//
// Router/Modem son *Segment (a diferencia de Internet/DNS, que siempre se
// intentan): nil significa "no se intentó medir este tramo en este ciclo"
// —Modem, cuando el anillo exterior todavía no se autodetectó (ver
// outerRingConfig en cmd/vigia-agent/router.go); Router, en el caso límite
// de que el agente arranque sin gateway conocido todavía. Esta distinción
// es la que separa BlameUnknown ("no se sabe": Router == nil) de BlameLAN
// ("se sabe que el router no contesta": Router != nil && !Router.OK) — ver
// blameFor en outage.go.
type Sample struct {
	At       time.Time
	Router   *Segment
	Modem    *Segment
	Internet Segment
	DNS      Segment
}

// Outage es una caída ya detectada (ver docs/agente-api.md, "Caídas").
// End == nil mientras siga abierta.
type Outage struct {
	Start time.Time
	End   *time.Time
	Blame Blame
}

// DurationSec: cuánto duró (o lleva durando, si End es nil) la caída,
// medida contra `now` para una que siga abierta.
func (o Outage) DurationSec(now time.Time) float64 {
	end := now
	if o.End != nil {
		end = *o.End
	}
	return end.Sub(o.Start).Seconds()
}

// SegmentStats resume un tramo sobre una ventana (minuto u hora, ver Agg):
// enviados, perdidos, mín/prom/máx (ver docs/agente-api.md, "Se guarda: por
// minuto... enviados, perdidos, mín/prom/máx"). Min/Avg/Max solo cuentan
// las mediciones que SÍ contestaron (Sent-Lost de ellas); da igual si
// Sent == 0 (tramo nunca medido en la ventana, p. ej. el módem antes de
// autodetectarse) que si Sent == Lost (medido, pero nunca contestó): en
// los dos casos no hay un tiempo de respuesta real que promediar, así que
// Min/Avg/Max se quedan en 0 — HasLatency() es la forma correcta de
// distinguir "0 ms de verdad" de "sin dato".
type SegmentStats struct {
	Sent  int
	Lost  int
	MinMs float64
	AvgMs float64
	MaxMs float64
}

// HasLatency: true si al menos una medición de la ventana contestó (ver el
// comentario del tipo). Quien sirve la API usa esto para decidir "null" en
// vez de 0.
func (s SegmentStats) HasLatency() bool { return s.Sent > s.Lost }

// LossPct: porcentaje de pérdida del tramo en la ventana. 0 si Sent == 0
// (nunca se intentó: no hay pérdida que reportar, a diferencia de "se
// intentó y todo falló", que da 100).
func (s SegmentStats) LossPct() float64 {
	if s.Sent == 0 {
		return 0
	}
	return float64(s.Lost) / float64(s.Sent) * 100
}

// Agg es una ventana ya cerrada del medidor (un minuto o una hora, ver
// Window en aggregate.go): mismo shape para las dos — MinuteAgg/HourAgg son
// alias por claridad en las firmas de store/api, no tipos distintos.
type Agg struct {
	At       time.Time
	Router   SegmentStats
	Modem    SegmentStats
	Internet SegmentStats
	DNS      SegmentStats
}

// MinuteAgg y HourAgg: ver el comentario de Agg.
type MinuteAgg = Agg
type HourAgg = Agg

// SpeedTestResult es el resultado de una prueba de velocidad (ver
// docs/agente-api.md, "Prueba de velocidad" y GET /api/v1/internet,
// "speedTests"). Error != nil si la prueba falló (timeout, servidor no
// disponible...): en ese caso los demás campos numéricos se quedan en 0 y
// no se deben mostrar como si fueran un resultado real.
type SpeedTestResult struct {
	At              time.Time
	DownMbps        float64
	UpMbps          float64
	LatencyMs       float64
	LoadedLatencyMs float64
	Error           *string
}

// Settings son los ajustes de GET/PUT /api/v1/internet/settings (ver
// docs/agente-api.md).
type Settings struct {
	SpeedTestsPerDay int `json:"speedTestsPerDay"`
}

// DefaultSettings: 2 pruebas de velocidad al día (ver docs/agente-api.md:
// "Por defecto 2 al día").
func DefaultSettings() Settings {
	return Settings{SpeedTestsPerDay: 2}
}

// MaxSpeedTestsPerDay: tope del ajuste (ver docs/agente-api.md: "configurable
// de 0 a 6").
const MaxSpeedTestsPerDay = 6

// Valid: true si SpeedTestsPerDay está en [0, MaxSpeedTestsPerDay].
func (s Settings) Valid() bool {
	return s.SpeedTestsPerDay >= 0 && s.SpeedTestsPerDay <= MaxSpeedTestsPerDay
}
