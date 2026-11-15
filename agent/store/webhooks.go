package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Webhook es un destino de alertas registrado (ver docs/agente-api.md,
// sección Webhooks). Secret se guarda para firmar X-Vigia-Signature pero
// jamás se sirve por la API: el tag json:"-" lo excluye por construcción,
// sin que cada handler tenga que acordarse de ocultarlo a mano (ver
// "sin secret en la respuesta" en el contrato).
type Webhook struct {
	ID         int64     `json:"id"`
	URL        string    `json:"url"`
	Types      []string  `json:"types"`
	Secret     string    `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
	LastStatus *int      `json:"lastStatus"`
	LastError  *string   `json:"lastError"`
}

// Accepts: true si typ debe mandarse a este webhook. Types vacío = todos
// (ver "Filtro por types" en docs/agente-api.md).
func (w Webhook) Accepts(typ string) bool {
	if len(w.Types) == 0 {
		return true
	}
	for _, t := range w.Types {
		if t == typ {
			return true
		}
	}
	return false
}

func encodeTypes(types []string) *string {
	return nilIfEmpty(strings.Join(types, ","))
}

func decodeTypes(csv *string) []string {
	if csv == nil || *csv == "" {
		return nil
	}
	return strings.Split(*csv, ",")
}

const webhookColumns = `id, url, types, secret, created_at, last_status, last_error`

func scanWebhook(scan func(dest ...any) error) (Webhook, error) {
	var w Webhook
	var types, secret, lastError *string
	var createdAt string
	if err := scan(&w.ID, &w.URL, &types, &secret, &createdAt, &w.LastStatus, &lastError); err != nil {
		return Webhook{}, err
	}
	w.Types = decodeTypes(types)
	if secret != nil {
		w.Secret = *secret
	}
	w.LastError = lastError
	var err error
	if w.CreatedAt, err = parseTime(createdAt); err != nil {
		return Webhook{}, err
	}
	return w, nil
}

// CreateWebhook registra un webhook nuevo (ver POST /api/v1/webhooks).
func (s *Store) CreateWebhook(ctx context.Context, url string, types []string, secret string, at time.Time) (Webhook, error) {
	return withTx(ctx, s.db, func(tx *sql.Tx) (Webhook, error) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO webhooks (url, types, secret, created_at) VALUES (?, ?, ?, ?)
		`, url, encodeTypes(types), nilIfEmpty(secret), formatTime(at))
		if err != nil {
			return Webhook{}, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return Webhook{}, err
		}
		return Webhook{ID: id, URL: url, Types: types, Secret: secret, CreatedAt: at}, nil
	})
}

// Webhooks lista todos los webhooks registrados, por fecha de alta (ver
// GET /api/v1/webhooks).
func (s *Store) Webhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+webhookColumns+` FROM webhooks ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Webhook
	for rows.Next() {
		w, err := scanWebhook(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// WebhookByID devuelve el webhook id, o ErrNotFound si no existe. Como
// Webhooks, trae el secreto en el valor Go (lo necesita quien vaya a
// firmar una entrega); nunca se expone por la API — ver json:"-" en
// Webhook, que es la frontera real de seguridad, no qué método del store
// se llamó.
func (s *Store) WebhookByID(ctx context.Context, id int64) (Webhook, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+webhookColumns+` FROM webhooks WHERE id = ?`, id)
	w, err := scanWebhook(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Webhook{}, ErrNotFound
	}
	return w, err
}

// DeleteWebhook borra el webhook id (ver DELETE /api/v1/webhooks/{id}).
// ErrNotFound si no existe.
func (s *Store) DeleteWebhook(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateWebhookResult guarda el desenlace de la última entrega (ver "se
// guarda el último estado" en docs/agente-api.md). status nil + errMsg
// no nil: la petición no se pudo completar (red, timeout). status no
// nil: el servidor remoto respondió ese código, sin importar si fue 2xx
// — errMsg va aparte, solo para fallos de transporte.
func (s *Store) UpdateWebhookResult(ctx context.Context, id int64, status *int, errMsg *string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhooks SET last_status = ?, last_error = ? WHERE id = ?`, status, errMsg, id)
	return err
}
