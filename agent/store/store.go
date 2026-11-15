// Package store persiste redes, equipos, su cronología de eventos, los
// clientes vinculados (solo el hash del token) y los códigos de vinculación
// pendientes, en SQLite vía modernc.org/sqlite (Go puro, sin cgo). Esquema
// versionado con PRAGMA user_version; ver migrations.go.
//
// Decisión — una sola conexión: PRAGMA foreign_keys y busy_timeout son por
// CONEXIÓN, no por archivo. Con el pool normal de *sql.DB no hay garantía de
// que se apliquen a la conexión que de verdad ejecuta cada consulta (podría
// abrir una nueva sin esos PRAGMA). db.SetMaxOpenConns(1) fuerza una sola
// conexión por proceso: se configuran una vez y valen siempre. El agente es
// de carga baja (un escaneo cada 5 min, un puñado de clientes HTTP) así que
// no hace falta más concurrencia que esa. journal_mode=WAL + busy_timeout
// siguen permitiendo que la CLI (tokens/pair/revoke) abra el mismo archivo
// desde OTRO proceso sin pisarse con el agente corriendo.
//
// Decisión — fechas: se guardan como TEXT en RFC 3339 UTC, formateadas y
// parseadas a mano (formatTime/parseTime), nunca pasando un time.Time
// directo como argumento ni escaneando directo a *time.Time: el formato por
// defecto del driver al bindear un time.Time es time.Time.String() (no RFC
// 3339), que ni ordena bien como texto ni es el formato que exige el
// contrato. Guardando siempre el string ya formateado evitamos depender de
// ese detalle del driver.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// Store envuelve la base SQLite del agente.
type Store struct {
	db *sql.DB
}

// querier es lo que *sql.DB y *sql.Tx tienen en común: deja escribir
// funciones que corren igual sueltas que dentro de una transacción.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Open abre (y crea si falta) la base en path, aplica las migraciones
// pendientes y la deja lista para usar. El directorio que contiene path
// debe existir ya con los permisos del contrato (0700); Open solo fija los
// del archivo (0600) — crear el directorio es cosa de quien resuelve las
// rutas (ver agent.Paths en el paquete agent).
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: abrir %s: %w", path, err)
	}
	// Una sola conexión: ver comentario del paquete.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: %s: %w", pragma, err)
		}
	}

	if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
		db.Close()
		return nil, fmt.Errorf("store: permisos de %s: %w", path, err)
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migración: %w", err)
	}
	return s, nil
}

// Close cierra la base.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		if err := s.applyMigration(ctx, i); err != nil {
			return fmt.Errorf("migración %d: %w", i+1, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, i int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
		tx.Rollback()
		return err
	}
	// PRAGMA user_version no admite "?": el número va en el texto del SQL.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// withTx corre fn dentro de una transacción: commit si no hay error,
// rollback si lo hay.
func withTx[T any](ctx context.Context, db *sql.DB, fn func(*sql.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	v, err := fn(tx)
	if err != nil {
		tx.Rollback()
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return v, nil
}

// --- fechas: ver la nota de decisión al principio del archivo. ---

const utcTimeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string { return t.UTC().Format(utcTimeLayout) }

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("fecha %q: %w", s, err)
	}
	return t.UTC(), nil
}

func parseTimePtr(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	t, err := parseTime(*s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
