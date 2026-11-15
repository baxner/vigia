package store

import (
	"context"
	"database/sql"

	"github.com/baxner/vigia/agent/history"
)

func insertEvent(ctx context.Context, tx *sql.Tx, ev history.DeviceEvent) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO device_events (device_id, type, at, detail) VALUES (?, ?, ?, ?)
	`, ev.DeviceID, string(ev.Type), formatTime(ev.At), ev.Detail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Events devuelve la cronología de deviceID, del más reciente al más
// antiguo, acotada a limit (si limit <= 0, se usa 200: el valor por
// defecto del contrato para GET /api/v1/devices/{id}/events).
func (s *Store) Events(ctx context.Context, deviceID int64, limit int) ([]history.DeviceEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT device_id, type, at, detail FROM device_events
		WHERE device_id = ? ORDER BY at DESC, id DESC LIMIT ?
	`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []history.DeviceEvent
	for rows.Next() {
		ev, err := scanEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func scanEvent(scan func(dest ...any) error) (history.DeviceEvent, error) {
	var ev history.DeviceEvent
	var typ, at string
	if err := scan(&ev.DeviceID, &typ, &at, &ev.Detail); err != nil {
		return history.DeviceEvent{}, err
	}
	ev.Type = history.DeviceEventType(typ)
	var err error
	if ev.At, err = parseTime(at); err != nil {
		return history.DeviceEvent{}, err
	}
	return ev, nil
}
