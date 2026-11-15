package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/baxner/vigia/agent/alerts"
)

// Alert es una alerta ya persistida (ver GET /api/v1/alerts en
// docs/agente-api.md); los tags JSON son el contrato tal cual, igual que
// store.Network y los demás tipos que la API sirve sin un DTO aparte.
type Alert struct {
	ID       int64           `json:"id"`
	Type     alerts.Type     `json:"type"`
	Severity alerts.Severity `json:"severity"`
	At       time.Time       `json:"at"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	DeviceID *int64          `json:"deviceId"`
	Network  string          `json:"network"`
	Read     bool            `json:"read"`
}

const alertColumns = `id, type, severity, at, title, body, device_id, network_key, read`

func scanAlert(scan func(dest ...any) error) (Alert, error) {
	var a Alert
	var typ, severity, at string
	var network *string
	if err := scan(&a.ID, &typ, &severity, &at, &a.Title, &a.Body, &a.DeviceID, &network, &a.Read); err != nil {
		return Alert{}, err
	}
	a.Type = alerts.Type(typ)
	a.Severity = alerts.Severity(severity)
	var err error
	if a.At, err = parseTime(at); err != nil {
		return Alert{}, fmt.Errorf("at: %w", err)
	}
	if network != nil {
		a.Network = *network
	}
	return a, nil
}

// InsertAlert persiste a (ver agent/alerts.EvaluateScan/EvaluateHealth).
// created es false si ya existía una alerta con el mismo DedupKey —
// reevaluar el mismo evento (reintento, reinicio a mitad de un escaneo)
// nunca la duplica (ver docs/agente-api.md, "Alertas": "el reenvío de un
// mismo evento no duplica alertas") — en ese caso el Alert devuelto es el
// zero-value: quien llama no debe publicarlo por SSE ni mandarlo a los
// webhooks, así que ni se molesta en leer la fila existente.
func (s *Store) InsertAlert(ctx context.Context, a alerts.Alert) (Alert, bool, error) {
	if a.DedupKey == "" {
		return Alert{}, false, errors.New("store: alerts.Alert sin DedupKey")
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO alerts (type, severity, at, title, body, device_id, network_key, read, dedup_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)
		ON CONFLICT(dedup_key) DO NOTHING
	`, string(a.Type), string(a.Severity), formatTime(a.At), a.Title, a.Body, a.DeviceID, nilIfEmpty(a.Network), a.DedupKey)
	if err != nil {
		return Alert{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Alert{}, false, err
	}
	if n == 0 {
		return Alert{}, false, nil
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Alert{}, false, err
	}
	return Alert{
		ID: id, Type: a.Type, Severity: a.Severity, At: a.At, Title: a.Title, Body: a.Body,
		DeviceID: a.DeviceID, Network: a.Network, Read: false,
	}, true, nil
}

// Alerts devuelve las alertas con id > after, de la más vieja a la más
// nueva (ver GET /api/v1/alerts?after=&limit=, "para ponerse al día").
// limit <= 0 usa 100 (el valor por defecto del contrato).
func (s *Store) Alerts(ctx context.Context, after int64, limit int) ([]Alert, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+alertColumns+` FROM alerts WHERE id > ? ORDER BY id ASC LIMIT ?
	`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Alert
	for rows.Next() {
		a, err := scanAlert(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RecentAlerts devuelve las alertas con At >= since, de la más vieja a la
// más nueva — para GET /api/v1/security (ver docs/agente-api.md:
// "recentAlerts: las alertas de las últimas 24 h" y
// core.BuildSecurityReport, que decide ROUTER_CHANGED/ARP_CONFLICT mirando
// esta ventana). A diferencia de Alerts (paginada por id, "ponerse al
// día"), esta es por tiempo: id y tiempo no siempre coinciden en el mismo
// orden si dos alertas comparten el mismo `at` de un escaneo, pero para una
// ventana de 24 horas no hace falta ese detalle.
func (s *Store) RecentAlerts(ctx context.Context, since time.Time) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+alertColumns+` FROM alerts WHERE at >= ? ORDER BY at ASC
	`, formatTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Alert
	for rows.Next() {
		a, err := scanAlert(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkAlertsRead marca como leídas todas las alertas con id <= upTo (ver
// POST /api/v1/alerts/read, {"upTo": <id>}).
func (s *Store) MarkAlertsRead(ctx context.Context, upTo int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET read = 1 WHERE id <= ?`, upTo)
	return err
}

// --- AlertSettings: una sola fila (id=1) con un interruptor por tipo. ---

// AlertSettings devuelve la configuración vigente (ver GET
// /api/v1/alert-settings).
func (s *Store) AlertSettings(ctx context.Context) (alerts.Settings, error) {
	var set alerts.Settings
	err := s.db.QueryRowContext(ctx, `
		SELECT new_device, important_offline, important_online, agent_blind, agent_recovered,
			router_changed, arp_conflict, port_mapping_added, risky_service,
			internet_down, internet_restored
		FROM alert_settings WHERE id = 1
	`).Scan(&set.NewDevice, &set.ImportantOffline, &set.ImportantOnline, &set.AgentBlind, &set.AgentRecovered,
		&set.RouterChanged, &set.ArpConflict, &set.PortMappingAdded, &set.RiskyService,
		&set.InternetDown, &set.InternetRestored)
	if errors.Is(err, sql.ErrNoRows) {
		// No debería pasar (migration2 inserta la fila), pero un agente
		// recién migrado no tiene por qué quedarse sin avisos.
		return alerts.DefaultSettings(), nil
	}
	return set, err
}

// SetAlertSettings reemplaza la configuración entera (ver PUT
// /api/v1/alert-settings: reemplazo completo, no un parche — a diferencia
// de PATCH /api/v1/devices/{id}, aquí no hay "campo ausente = sin
// cambio").
func (s *Store) SetAlertSettings(ctx context.Context, set alerts.Settings) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_settings SET new_device = ?, important_offline = ?, important_online = ?,
			agent_blind = ?, agent_recovered = ?,
			router_changed = ?, arp_conflict = ?, port_mapping_added = ?, risky_service = ?,
			internet_down = ?, internet_restored = ?
		WHERE id = 1
	`, set.NewDevice, set.ImportantOffline, set.ImportantOnline, set.AgentBlind, set.AgentRecovered,
		set.RouterChanged, set.ArpConflict, set.PortMappingAdded, set.RiskyService,
		set.InternetDown, set.InternetRestored)
	return err
}
