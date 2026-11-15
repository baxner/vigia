package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"time"
)

// Client es un teléfono (o cualquier otro consumidor) vinculado: el agente
// solo guarda el hash SHA-256 del token que le entregó una vez, nunca el
// token en claro (ver "Vinculación" en docs/agente-api.md).
type Client struct {
	ID         int64
	Name       string
	TokenHash  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	// NULL = vigente.
	RevokedAt *time.Time
}

// Active: no fue revocado.
func (c Client) Active() bool { return c.RevokedAt == nil }

// HashToken: SHA-256 en hex de un token en claro — lo único que el agente
// guarda de él.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateClient registra un cliente nuevo con el hash de su token (ver
// HashToken) y devuelve la fila creada.
func (s *Store) CreateClient(ctx context.Context, name, tokenHash string, at time.Time) (Client, error) {
	return withTx(ctx, s.db, func(tx *sql.Tx) (Client, error) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO clients (name, token_hash, created_at) VALUES (?, ?, ?)
		`, name, tokenHash, formatTime(at))
		if err != nil {
			return Client{}, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return Client{}, err
		}
		return Client{ID: id, Name: name, TokenHash: tokenHash, CreatedAt: at}, nil
	})
}

// Clients lista todos los clientes vinculados (vigentes y revocados), por
// fecha de alta — para `vigia-agent tokens`.
func (s *Store) Clients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, token_hash, created_at, last_used_at, revoked_at
		FROM clients ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Client
	for rows.Next() {
		c, err := scanClient(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ActiveClients devuelve solo los clientes vigentes (no revocados): es lo
// que compara el middleware de auth contra el token presentado.
func (s *Store) ActiveClients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, token_hash, created_at, last_used_at, revoked_at
		FROM clients WHERE revoked_at IS NULL ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Client
	for rows.Next() {
		c, err := scanClient(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanClient(scan func(dest ...any) error) (Client, error) {
	var c Client
	var createdAt string
	var lastUsedAt, revokedAt *string
	if err := scan(&c.ID, &c.Name, &c.TokenHash, &createdAt, &lastUsedAt, &revokedAt); err != nil {
		return Client{}, err
	}
	var err error
	if c.CreatedAt, err = parseTime(createdAt); err != nil {
		return Client{}, err
	}
	if c.LastUsedAt, err = parseTimePtr(lastUsedAt); err != nil {
		return Client{}, err
	}
	if c.RevokedAt, err = parseTimePtr(revokedAt); err != nil {
		return Client{}, err
	}
	return c, nil
}

// TouchClient actualiza lastUsedAt; se llama en cada request autenticado.
func (s *Store) TouchClient(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clients SET last_used_at = ? WHERE id = ?`, formatTime(at), id)
	return err
}

// RevokeByName revoca TODOS los clientes vigentes cuyo nombre sea
// exactamente name. Un mismo teléfono puede haberse vinculado más de una
// vez (p. ej. tras un reset de fábrica) y terminar con más de un token
// activo bajo el mismo nombre; "revocar este teléfono" debe quitarle el
// acceso a todos. Devuelve cuántos revocó (0 = no había ninguno vigente con
// ese nombre).
func (s *Store) RevokeByName(ctx context.Context, name string, at time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE clients SET revoked_at = ? WHERE name = ? AND revoked_at IS NULL
	`, formatTime(at), name)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// FindActiveClientByToken busca el cliente vigente cuyo hash coincida con
// el de token, comparando en tiempo constante (crypto/subtle) — no con
// "WHERE token_hash = ?" en SQL ni con "==" en Go, cuya igualdad de texto no
// es constante. nil sin error si ninguno coincide.
func (s *Store) FindActiveClientByToken(ctx context.Context, token string) (*Client, error) {
	want := sha256.Sum256([]byte(token))
	clients, err := s.ActiveClients(ctx)
	if err != nil {
		return nil, err
	}
	for i := range clients {
		got, err := hex.DecodeString(clients[i].TokenHash)
		if err != nil || len(got) != len(want) {
			continue
		}
		if subtle.ConstantTimeCompare(got, want[:]) == 1 {
			return &clients[i], nil
		}
	}
	return nil, nil
}
