package store

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"
)

const (
	pairingTTL         = 5 * time.Minute
	pairingMaxAttempts = 5
)

// PairingCode es un código de vinculación vigente (ver "Vinculación" en
// docs/agente-api.md). A lo sumo uno a la vez: generar uno nuevo invalida
// cualquier otro pendiente ("después, código nuevo").
type PairingCode struct {
	Code      string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// CreatePairingCode genera un código de 6 dígitos (con ceros a la
// izquierda) válido por 5 minutos y descarta cualquier código pendiente
// anterior.
func (s *Store) CreatePairingCode(ctx context.Context, at time.Time) (PairingCode, error) {
	return withTx(ctx, s.db, func(tx *sql.Tx) (PairingCode, error) {
		if _, err := tx.ExecContext(ctx, "DELETE FROM pairing_codes"); err != nil {
			return PairingCode{}, err
		}
		code, err := randomDigits(6)
		if err != nil {
			return PairingCode{}, err
		}
		pc := PairingCode{Code: code, CreatedAt: at, ExpiresAt: at.Add(pairingTTL)}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pairing_codes (code, created_at, expires_at, attempts) VALUES (?, ?, ?, 0)
		`, pc.Code, formatTime(pc.CreatedAt), formatTime(pc.ExpiresAt)); err != nil {
			return PairingCode{}, err
		}
		return pc, nil
	})
}

// PairingResult dice qué pasó con un intento de POST /api/v1/pair.
type PairingResult int

const (
	PairingOK                PairingResult = iota
	PairingNoActiveCode                    // no hay ningún código pendiente (nunca hubo, o ya se gastó)
	PairingExpired                         // el código pendiente venció (pasaron los 5 min)
	PairingAttemptsExhausted               // se agotaron los 5 intentos: hace falta un código nuevo
	PairingWrongCode                       // no coincide con el código pendiente
)

// TryConsumePairingCode es el único punto de entrada de POST /api/v1/pair:
// sin importar qué string llegue, siempre se compara contra el ÚNICO
// código pendiente (si hay uno) y SIEMPRE cuenta como intento — así alguien
// probando códigos al azar gasta el cupo de 5 igual que alguien tipeando
// mal el suyo (si solo contáramos intentos sobre el código exacto
// adivinado, probar códigos que nunca existieron no gastaría nada y el
// límite de intentos no protegería de nada). Al acertar, o al agotarse el
// cupo o el tiempo, el código pendiente se borra: de un solo uso.
func (s *Store) TryConsumePairingCode(ctx context.Context, code string, at time.Time) (PairingResult, error) {
	return withTx(ctx, s.db, func(tx *sql.Tx) (PairingResult, error) {
		var dbCode, expiresAt string
		var attempts int
		err := tx.QueryRowContext(ctx, `SELECT code, expires_at, attempts FROM pairing_codes LIMIT 1`).
			Scan(&dbCode, &expiresAt, &attempts)
		if errors.Is(err, sql.ErrNoRows) {
			return PairingNoActiveCode, nil
		}
		if err != nil {
			return PairingNoActiveCode, err
		}

		expiry, err := parseTime(expiresAt)
		if err != nil {
			return PairingNoActiveCode, err
		}
		if at.After(expiry) {
			_, err := tx.ExecContext(ctx, "DELETE FROM pairing_codes")
			return PairingExpired, err
		}

		attempts++
		if attempts > pairingMaxAttempts {
			_, err := tx.ExecContext(ctx, "DELETE FROM pairing_codes")
			return PairingAttemptsExhausted, err
		}

		if subtle.ConstantTimeCompare([]byte(dbCode), []byte(code)) != 1 {
			_, err := tx.ExecContext(ctx, "UPDATE pairing_codes SET attempts = ?", attempts)
			return PairingWrongCode, err
		}

		_, err = tx.ExecContext(ctx, "DELETE FROM pairing_codes")
		return PairingOK, err
	})
}

// randomDigits genera un código de n dígitos (con ceros a la izquierda),
// uniforme en [0, 10^n), con crypto/rand.
func randomDigits(n int) (string, error) {
	limit := big.NewInt(1)
	ten := big.NewInt(10)
	for i := 0; i < n; i++ {
		limit.Mul(limit, ten)
	}
	v, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("código aleatorio: %w", err)
	}
	return fmt.Sprintf("%0*d", n, v), nil
}
