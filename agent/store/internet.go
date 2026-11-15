package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/baxner/vigia/agent/internet"
)

// MinuteRetention/HourRetention: cuánto se guarda de la agregación por
// minuto y por hora (ver docs/agente-api.md, "Se guarda: por minuto... 7
// días, y por hora... 90 días"). RecordMinute/RecordHour podan lo viejo en
// cada escritura (ver el comentario de migration5): sin tabla ni job de
// limpieza aparte, igual de simple que "una conexión, un PRAGMA" en
// store.go.
const (
	MinuteRetention = 7 * 24 * time.Hour
	HourRetention   = 90 * 24 * time.Hour
)

// internetAggColumns: las 20 columnas (5 por tramo × 4 tramos) que
// comparten internet_minutes e internet_hours (ver migration5).
const internetAggColumns = `at,
	router_sent, router_lost, router_min_ms, router_avg_ms, router_max_ms,
	modem_sent, modem_lost, modem_min_ms, modem_avg_ms, modem_max_ms,
	internet_sent, internet_lost, internet_min_ms, internet_avg_ms, internet_max_ms,
	dns_sent, dns_lost, dns_min_ms, dns_avg_ms, dns_max_ms`

func scanInternetAgg(scan func(dest ...any) error) (internet.Agg, error) {
	var a internet.Agg
	var at string
	if err := scan(&at,
		&a.Router.Sent, &a.Router.Lost, &a.Router.MinMs, &a.Router.AvgMs, &a.Router.MaxMs,
		&a.Modem.Sent, &a.Modem.Lost, &a.Modem.MinMs, &a.Modem.AvgMs, &a.Modem.MaxMs,
		&a.Internet.Sent, &a.Internet.Lost, &a.Internet.MinMs, &a.Internet.AvgMs, &a.Internet.MaxMs,
		&a.DNS.Sent, &a.DNS.Lost, &a.DNS.MinMs, &a.DNS.AvgMs, &a.DNS.MaxMs,
	); err != nil {
		return internet.Agg{}, err
	}
	var err error
	if a.At, err = parseTime(at); err != nil {
		return internet.Agg{}, err
	}
	return a, nil
}

// RecordMinuteAgg persiste una ventana de un minuto ya cerrada (ver
// agent/internet.Window con bucket time.Minute, alimentada en
// cmd/vigia-agent/internet.go) y poda lo anterior a MinuteRetention.
// ON CONFLICT(at) DO UPDATE: defensivo ante un reinicio justo en el borde
// de un minuto que ya se había guardado (no debería pasar en operación
// normal: ver el comentario de Window.Feed).
func (s *Store) RecordMinuteAgg(ctx context.Context, a internet.Agg) error {
	return s.recordInternetAgg(ctx, "internet_minutes", a, time.Now().UTC().Add(-MinuteRetention))
}

// RecordHourAgg: como RecordMinuteAgg, pero en internet_hours y podando a
// HourRetention.
func (s *Store) RecordHourAgg(ctx context.Context, a internet.Agg) error {
	return s.recordInternetAgg(ctx, "internet_hours", a, time.Now().UTC().Add(-HourRetention))
}

func (s *Store) recordInternetAgg(ctx context.Context, table string, a internet.Agg, oldestToKeep time.Time) error {
	_, err := withTx(ctx, s.db, func(tx *sql.Tx) (struct{}, error) {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO `+table+` (`+internetAggColumns+`)
			VALUES (?, ?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?)
			ON CONFLICT(at) DO UPDATE SET
				router_sent = excluded.router_sent, router_lost = excluded.router_lost,
				router_min_ms = excluded.router_min_ms, router_avg_ms = excluded.router_avg_ms, router_max_ms = excluded.router_max_ms,
				modem_sent = excluded.modem_sent, modem_lost = excluded.modem_lost,
				modem_min_ms = excluded.modem_min_ms, modem_avg_ms = excluded.modem_avg_ms, modem_max_ms = excluded.modem_max_ms,
				internet_sent = excluded.internet_sent, internet_lost = excluded.internet_lost,
				internet_min_ms = excluded.internet_min_ms, internet_avg_ms = excluded.internet_avg_ms, internet_max_ms = excluded.internet_max_ms,
				dns_sent = excluded.dns_sent, dns_lost = excluded.dns_lost,
				dns_min_ms = excluded.dns_min_ms, dns_avg_ms = excluded.dns_avg_ms, dns_max_ms = excluded.dns_max_ms
		`, formatTime(a.At),
			a.Router.Sent, a.Router.Lost, a.Router.MinMs, a.Router.AvgMs, a.Router.MaxMs,
			a.Modem.Sent, a.Modem.Lost, a.Modem.MinMs, a.Modem.AvgMs, a.Modem.MaxMs,
			a.Internet.Sent, a.Internet.Lost, a.Internet.MinMs, a.Internet.AvgMs, a.Internet.MaxMs,
			a.DNS.Sent, a.DNS.Lost, a.DNS.MinMs, a.DNS.AvgMs, a.DNS.MaxMs,
		)
		if err != nil {
			return struct{}{}, err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE at < ?`, formatTime(oldestToKeep))
		return struct{}{}, err
	})
	return err
}

// MinuteSeries devuelve las ventanas de un minuto con at >= since, de la
// más vieja a la más nueva (ver GET /api/v1/internet/series?range=24h).
func (s *Store) MinuteSeries(ctx context.Context, since time.Time) ([]internet.Agg, error) {
	return s.internetSeries(ctx, "internet_minutes", since)
}

// HourSeries: como MinuteSeries, pero de internet_hours (ver GET
// /api/v1/internet/series?range=7d).
func (s *Store) HourSeries(ctx context.Context, since time.Time) ([]internet.Agg, error) {
	return s.internetSeries(ctx, "internet_hours", since)
}

func (s *Store) internetSeries(ctx context.Context, table string, since time.Time) ([]internet.Agg, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+internetAggColumns+` FROM `+table+` WHERE at >= ? ORDER BY at ASC
	`, formatTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []internet.Agg
	for rows.Next() {
		a, err := scanInternetAgg(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RecentMinutes devuelve las últimas `window` de ventanas por minuto ya
// cerradas, de la más vieja a la más nueva — para calcular la mediana de
// 24 h que necesita agent/internet.EvaluateState (ver
// cmd/vigia-agent/internet.go).
func (s *Store) RecentMinutes(ctx context.Context, window time.Duration) ([]internet.Agg, error) {
	return s.MinuteSeries(ctx, time.Now().UTC().Add(-window))
}

// --- Outages (ver docs/agente-api.md, "Caídas"). ---

// OutageRow es una caída tal como la sirve la API (ver GET
// /api/v1/internet, "outages"): mismos campos que agent/internet.Outage
// más el id de persistencia (que ese tipo no tiene, al ser puro).
type OutageRow struct {
	ID          int64          `json:"id"`
	Start       time.Time      `json:"start"`
	End         *time.Time     `json:"end"`
	DurationSec float64        `json:"durationSec"`
	Blame       internet.Blame `json:"blame"`
}

func outageRowFrom(id int64, o internet.Outage, now time.Time) OutageRow {
	return OutageRow{ID: id, Start: o.Start, End: o.End, DurationSec: o.DurationSec(now), Blame: o.Blame}
}

// InsertOutageOpen persiste una caída recién ABIERTA (ver
// agent/internet.OutageDetector.Feed, cuando devuelve una Outage con
// End == nil) y devuelve su id, para que CloseOutage pueda cerrar
// exactamente esa fila sin ambigüedad aunque dos caídas compartieran el
// mismo Start (no debería pasar: OutageDetector es un solo detector en
// memoria con a lo sumo una caída abierta a la vez).
func (s *Store) InsertOutageOpen(ctx context.Context, o internet.Outage) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO internet_outages (start, end, blame) VALUES (?, NULL, ?)
	`, formatTime(o.Start), string(o.Blame))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CloseOutage pone fin a la fila id (ver
// agent/internet.OutageDetector.Feed, cuando devuelve una Outage con
// End != nil). No falla si id no existe (defensivo: un id inválido no
// debe tirar abajo el ciclo del medidor).
func (s *Store) CloseOutage(ctx context.Context, id int64, end time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE internet_outages SET end = ? WHERE id = ?`, formatTime(end), id)
	return err
}

// OpenOutage devuelve la caída abierta ahora (end IS NULL), o
// ErrNotFound si no hay ninguna — para que cmd/vigia-agent/internet.go
// pueda RESUMIR la fila (no los contadores, ver el comentario de
// OutageDetector) al arrancar, si el proceso anterior se cerró a mitad de
// una caída.
func (s *Store) OpenOutage(ctx context.Context) (int64, internet.Outage, error) {
	var id int64
	var start string
	var blame string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, start, blame FROM internet_outages WHERE end IS NULL ORDER BY id DESC LIMIT 1
	`).Scan(&id, &start, &blame)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, internet.Outage{}, ErrNotFound
	}
	if err != nil {
		return 0, internet.Outage{}, err
	}
	startAt, err := parseTime(start)
	if err != nil {
		return 0, internet.Outage{}, err
	}
	return id, internet.Outage{Start: startAt, Blame: internet.Blame(blame)}, nil
}

// RecentOutages devuelve las caídas con Start >= since, la más nueva
// primero (ver docs/agente-api.md: "últimos 30 días, la más nueva
// primero").
func (s *Store) RecentOutages(ctx context.Context, since time.Time) ([]OutageRow, error) {
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, start, end, blame FROM internet_outages WHERE start >= ? ORDER BY start DESC
	`, formatTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OutageRow
	for rows.Next() {
		var id int64
		var start string
		var end *string
		var blame string
		if err := rows.Scan(&id, &start, &end, &blame); err != nil {
			return nil, err
		}
		startAt, err := parseTime(start)
		if err != nil {
			return nil, err
		}
		endAt, err := parseTimePtr(end)
		if err != nil {
			return nil, err
		}
		out = append(out, outageRowFrom(id, internet.Outage{Start: startAt, End: endAt, Blame: internet.Blame(blame)}, now))
	}
	return out, rows.Err()
}

// --- Pruebas de velocidad (ver docs/agente-api.md, "Prueba de velocidad"). ---

// SpeedTestRow es una prueba de velocidad ya persistida (ver GET
// /api/v1/internet, "speedTests").
type SpeedTestRow struct {
	ID              int64     `json:"id"`
	At              time.Time `json:"at"`
	DownMbps        float64   `json:"downMbps"`
	UpMbps          float64   `json:"upMbps"`
	LatencyMs       float64   `json:"latencyMs"`
	LoadedLatencyMs float64   `json:"loadedLatencyMs"`
	Error           *string   `json:"error"`
}

// RecordSpeedTest persiste un resultado (exitoso o fallido, ver
// internet.SpeedTestResult.Error) de POST /api/v1/internet/speedtest o de
// una corrida programada.
func (s *Store) RecordSpeedTest(ctx context.Context, r internet.SpeedTestResult) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO internet_speed_tests (at, down_mbps, up_mbps, latency_ms, loaded_latency_ms, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`, formatTime(r.At), r.DownMbps, r.UpMbps, r.LatencyMs, r.LoadedLatencyMs, r.Error)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RecentSpeedTests devuelve las últimas `limit` pruebas, la más nueva
// primero (ver docs/agente-api.md: "últimas 30"). limit <= 0 usa 30.
func (s *Store) RecentSpeedTests(ctx context.Context, limit int) ([]SpeedTestRow, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, at, down_mbps, up_mbps, latency_ms, loaded_latency_ms, error
		FROM internet_speed_tests ORDER BY at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SpeedTestRow
	for rows.Next() {
		var r SpeedTestRow
		var at string
		if err := rows.Scan(&r.ID, &at, &r.DownMbps, &r.UpMbps, &r.LatencyMs, &r.LoadedLatencyMs, &r.Error); err != nil {
			return nil, err
		}
		if r.At, err = parseTime(at); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Ajustes de Internet (ver GET/PUT /api/v1/internet/settings). ---

// InternetSettings devuelve la configuración vigente.
func (s *Store) InternetSettings(ctx context.Context) (internet.Settings, error) {
	var set internet.Settings
	err := s.db.QueryRowContext(ctx, `SELECT speed_tests_per_day FROM internet_settings WHERE id = 1`).
		Scan(&set.SpeedTestsPerDay)
	if errors.Is(err, sql.ErrNoRows) {
		// No debería pasar (migration5 inserta la fila): mismo criterio
		// defensivo que AlertSettings.
		return internet.DefaultSettings(), nil
	}
	return set, err
}

// SetInternetSettings reemplaza la configuración (ver PUT
// /api/v1/internet/settings). Quien llama (agent/api) ya validó
// set.Valid() antes de llegar aquí.
func (s *Store) SetInternetSettings(ctx context.Context, set internet.Settings) error {
	_, err := s.db.ExecContext(ctx, `UPDATE internet_settings SET speed_tests_per_day = ? WHERE id = 1`, set.SpeedTestsPerDay)
	return err
}
