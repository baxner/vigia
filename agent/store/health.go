package store

import (
	"context"
	"time"
)

// AgentHealthState devuelve si el agente está marcado como "ciego" (ver
// AGENT_BLIND en docs/agente-api.md) ANTES de evaluar este escaneo —
// persistido para sobrevivir reinicios: scanHealth en
// cmd/vigia-agent/run.go es solo en memoria y sirve para
// GET /api/v1/info, pero decidir si ya toca disparar AGENT_BLIND (o
// AGENT_RECOVERED) necesita sobrevivir un reinicio del proceso. La fila
// (id=1) la inserta migration2; no debería faltar nunca.
func (s *Store) AgentHealthState(ctx context.Context) (bool, error) {
	var blind bool
	err := s.db.QueryRowContext(ctx, `SELECT blind FROM agent_health WHERE id = 1`).Scan(&blind)
	return blind, err
}

// SetAgentHealthState persiste el nuevo estado. Se llama en cada escaneo
// en el que agent/alerts.EvaluateHealth marcó transitioned=true — SIEMPRE,
// sin importar si AGENT_BLIND/AGENT_RECOVERED están apagados en
// alert_settings: perder este estado dejaría una transición sin
// registrar para cuando el ajuste se reactive.
func (s *Store) SetAgentHealthState(ctx context.Context, blind bool, network string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_health SET blind = ?, network_key = ?, since = ? WHERE id = 1`,
		blind, nilIfEmpty(network), formatTime(at))
	return err
}
