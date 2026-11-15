package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/baxner/vigia/agent/internet"
)

// outagesWindow: "últimos 30 días" (ver docs/agente-api.md, GET
// /api/v1/internet, "outages").
const outagesWindow = 30 * 24 * time.Hour

// speedTestsLimit: "últimas 30" (ver docs/agente-api.md, "speedTests").
const speedTestsLimit = 30

// nowSnapshot: "now" de InternetStatus (ver docs/agente-api.md).
type nowSnapshot struct {
	RouterMs   float64 `json:"routerMs"`
	ModemMs    float64 `json:"modemMs"`
	InternetMs float64 `json:"internetMs"`
	DNSMs      float64 `json:"dnsMs"`
	LossPct    float64 `json:"lossPct"`
}

// outageView: un elemento de "outages" (ver docs/agente-api.md:
// '{"start","end"|null,"durationSec","blame"}' — sin id: no es un
// recurso direccionable por la API, a diferencia de, por ejemplo, un
// webhook).
type outageView struct {
	Start       time.Time      `json:"start"`
	End         *time.Time     `json:"end"`
	DurationSec float64        `json:"durationSec"`
	Blame       internet.Blame `json:"blame"`
}

// speedTestView: un elemento de "speedTests" (ver docs/agente-api.md:
// mismos campos que internet.SpeedTestResult, sin id).
type speedTestView struct {
	At              time.Time `json:"at"`
	DownMbps        float64   `json:"downMbps"`
	UpMbps          float64   `json:"upMbps"`
	LatencyMs       float64   `json:"latencyMs"`
	LoadedLatencyMs float64   `json:"loadedLatencyMs"`
	Error           *string   `json:"error"`
}

// internetStatusResponse: InternetStatus tal cual el contrato (ver
// docs/agente-api.md, GET /api/v1/internet).
type internetStatusResponse struct {
	State      internet.State      `json:"state"`
	Since      time.Time           `json:"since"`
	Now        nowSnapshot         `json:"now"`
	IPv6       internet.IPv6Status `json:"ipv6"`
	Outages    []outageView        `json:"outages"`
	SpeedTests []speedTestView     `json:"speedTests"`
}

func (s *Server) handleInternet(w http.ResponseWriter, r *http.Request) {
	if s.internet == nil {
		writeError(w, http.StatusServiceUnavailable, "el medidor de internet no está disponible")
		return
	}

	state, since, now, ipv6 := s.internet.InternetNow()

	outageRows, err := s.store.RecentOutages(r.Context(), time.Now().UTC().Add(-outagesWindow))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	outages := make([]outageView, len(outageRows))
	for i, o := range outageRows {
		outages[i] = outageView{Start: o.Start, End: o.End, DurationSec: o.DurationSec, Blame: o.Blame}
	}

	speedRows, err := s.store.RecentSpeedTests(r.Context(), speedTestsLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	speedTests := make([]speedTestView, len(speedRows))
	for i, t := range speedRows {
		speedTests[i] = speedTestView{
			At: t.At, DownMbps: t.DownMbps, UpMbps: t.UpMbps,
			LatencyMs: t.LatencyMs, LoadedLatencyMs: t.LoadedLatencyMs, Error: t.Error,
		}
	}

	resp := internetStatusResponse{
		State: state, Since: since,
		Now: nowSnapshot{
			RouterMs:   segmentMS(now.Router),
			ModemMs:    segmentMS(now.Modem),
			InternetMs: now.Internet.MS,
			DNSMs:      now.DNS.MS,
			LossPct:    lossPctOf(now.Internet),
		},
		IPv6:       ipv6,
		Outages:    outages,
		SpeedTests: speedTests,
	}
	writeJSON(w, http.StatusOK, resp)
}

// segmentMS: el ms de un Segment opcional (ver internet.Sample.Router/Modem):
// 0 si nil (tramo no medido todavía, p. ej. el módem antes de
// autodetectar el anillo exterior) o si falló (sin un tiempo de respuesta
// real que mostrar).
func segmentMS(seg *internet.Segment) float64 {
	if seg == nil || !seg.OK {
		return 0
	}
	return seg.MS
}

// lossPctOf: 0 % si el último tramo "internet" medido contestó, 100 % si
// no — es solo el "now" instantáneo (a diferencia del lossPct de un
// Agg, que es sobre una ventana completa).
func lossPctOf(seg internet.Segment) float64 {
	if seg.OK {
		return 0
	}
	return 100
}

// nullableMS: el promedio de un SegmentStats, o nil si la ventana no tiene
// ninguna medición que haya contestado (ver SegmentStats.HasLatency) —
// "null donde no hubo dato" del contrato de la serie.
func nullableMS(st internet.SegmentStats) *float64 {
	if !st.HasLatency() {
		return nil
	}
	v := st.AvgMs
	return &v
}

// seriesPointView: un elemento de GET /api/v1/internet/series (ver
// docs/agente-api.md).
type seriesPointView struct {
	At         time.Time `json:"at"`
	RouterMs   *float64  `json:"routerMs"`
	ModemMs    *float64  `json:"modemMs"`
	InternetMs *float64  `json:"internetMs"`
	DNSMs      *float64  `json:"dnsMs"`
	LossPct    float64   `json:"lossPct"`
}

func seriesPointFromAgg(a internet.Agg) seriesPointView {
	return seriesPointView{
		At: a.At, RouterMs: nullableMS(a.Router), ModemMs: nullableMS(a.Modem),
		InternetMs: nullableMS(a.Internet), DNSMs: nullableMS(a.DNS), LossPct: a.Internet.LossPct(),
	}
}

func seriesPointEmpty(at time.Time) seriesPointView {
	return seriesPointView{At: at}
}

// buildSeries rellena los huecos de rows (ver docs/agente-api.md: "null
// donde no hubo dato") entre since y until, cada step: una fila ausente en
// la base (el agente estuvo apagado, o esa ventana todavía no cerró) se
// sirve igual, con todos los campos de latencia en null y lossPct en 0 —
// la ausencia de la FILA completa es, para quien consume la serie, el
// mismo "sin dato" que la ausencia de un campo dentro de una fila que sí
// existe.
func buildSeries(rows []internet.Agg, since, until time.Time, step time.Duration) []seriesPointView {
	byAt := make(map[int64]internet.Agg, len(rows))
	for _, r := range rows {
		byAt[r.At.Unix()] = r
	}
	since, until = since.UTC().Truncate(step), until.UTC().Truncate(step)

	out := make([]seriesPointView, 0, int(until.Sub(since)/step)+1)
	for t := since; !t.After(until); t = t.Add(step) {
		if agg, ok := byAt[t.Unix()]; ok {
			out = append(out, seriesPointFromAgg(agg))
		} else {
			out = append(out, seriesPointEmpty(t))
		}
	}
	return out
}

// handleInternetSeries sirve GET /api/v1/internet/series?range=24h|7d (ver
// docs/agente-api.md): 24h es por minuto (agent/store.MinuteSeries), 7d es
// por hora (HourSeries).
func (s *Server) handleInternetSeries(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "24h"
	}

	now := time.Now().UTC()
	var (
		rows  []internet.Agg
		err   error
		step  time.Duration
		span  time.Duration
		since time.Time
	)
	switch rangeParam {
	case "24h":
		step, span = time.Minute, 24*time.Hour
		since = now.Add(-span).Truncate(step)
		rows, err = s.store.MinuteSeries(r.Context(), since)
	case "7d":
		step, span = time.Hour, 7*24*time.Hour
		since = now.Add(-span).Truncate(step)
		rows, err = s.store.HourSeries(r.Context(), since)
	default:
		writeError(w, http.StatusBadRequest, "range inválido: use 24h o 7d")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusOK, buildSeries(rows, since, now, step))
}

// handleInternetSpeedtest sirve POST /api/v1/internet/speedtest (ver
// docs/agente-api.md: "202 {\"started\": true}, o 409 si ya hay una
// corriendo").
func (s *Server) handleInternetSpeedtest(w http.ResponseWriter, r *http.Request) {
	if s.internet == nil {
		writeError(w, http.StatusServiceUnavailable, "el medidor de internet no está disponible")
		return
	}
	if !s.internet.TriggerSpeedTest() {
		writeError(w, http.StatusConflict, "ya hay una prueba de velocidad en curso")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

func (s *Server) handleInternetSettingsGet(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.InternetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// handleInternetSettingsPut sirve PUT /api/v1/internet/settings (ver
// docs/agente-api.md: "speedTestsPerDay... configurable de 0 a 6").
func (s *Server) handleInternetSettingsPut(w http.ResponseWriter, r *http.Request) {
	var set internet.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&set); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	if !set.Valid() {
		writeError(w, http.StatusBadRequest, "speedTestsPerDay debe estar entre 0 y 6")
		return
	}
	if err := s.store.SetInternetSettings(r.Context(), set); err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusOK, set)
}
