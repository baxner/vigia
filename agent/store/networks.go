package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Network es una red vigilada. Los tags JSON son el contrato tal cual (ver
// GET /api/v1/networks en docs/agente-api.md): el paquete api la sirve
// directo, sin un DTO aparte.
type Network struct {
	Key        string     `json:"key"`
	Subnet     string     `json:"subnet"`
	Gateway    string     `json:"gateway"`
	GatewayMAC *string    `json:"gatewayMac"`
	FirstSeen  time.Time  `json:"firstSeen"`
	LastSeen   time.Time  `json:"lastSeen"`
	LastScanAt *time.Time `json:"lastScanAt"`
	// Role (v1.2, ver docs/agente-api.md, "Seguridad" > Anillos): "inner"
	// (la red del router de casa) u "outer" (la de delante del router).
	// Siempre viene lleno desde este agente (nunca ""): defaultRole lo
	// reduce a "inner" si quien llama a touchNetwork no lo especifica (los
	// tests viejos de antes de la Fase 5, p. ej., no lo hacen).
	Role string `json:"role"`
}

// defaultRole: "" (quien llama no especificó rol) se trata como "inner" —
// el único rol que existía antes de la Fase 5 (ver NetworkInfo.Role en
// devices.go).
func defaultRole(role string) string {
	if role == "" {
		return "inner"
	}
	return role
}

// touchNetwork inserta la red si es la primera vez que se ve, o actualiza
// subnet/gateway/gatewayMAC/lastSeen/role si ya existía (el router se puede
// reemplazar, el DHCP puede reasignar la subred). role siempre se
// re-escribe con el mismo valor que ya tenía en la práctica: una misma
// clave (gateway|subred) nunca cambia de anillo entre un escaneo y el
// siguiente, así que no hace falta "no tocar role en un UPDATE" como caso
// aparte. lastScanAt se deja para markScanned, al terminar de reconciliar:
// si algo falla a mitad de RecordScan, no queremos que la red diga que el
// escaneo terminó.
func touchNetwork(ctx context.Context, tx *sql.Tx, key, subnet, gateway string, gatewayMAC *string, role string, at time.Time) error {
	atStr := formatTime(at)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO networks (key, subnet, gateway, gateway_mac, role, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			subnet = excluded.subnet,
			gateway = excluded.gateway,
			gateway_mac = excluded.gateway_mac,
			role = excluded.role,
			last_seen = excluded.last_seen
	`, key, subnet, gateway, gatewayMAC, defaultRole(role), atStr, atStr)
	return err
}

// markScanned actualiza lastScanAt de la red tras terminar de reconciliar
// un escaneo completo.
func markScanned(ctx context.Context, tx *sql.Tx, key string, at time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE networks SET last_scan_at = ? WHERE key = ?`, formatTime(at), key)
	return err
}

const networkColumns = `key, subnet, gateway, gateway_mac, role, first_seen, last_seen, last_scan_at`

// Networks devuelve todas las redes conocidas (interiores y exteriores,
// ver Network.Role), ordenadas por clave.
func (s *Store) Networks(ctx context.Context) ([]Network, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+networkColumns+` FROM networks ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Network
	for rows.Next() {
		n, err := scanNetwork(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// NetworkByKey devuelve la red key, o ErrNotFound si no existe (ver
// agent/alerts.EvaluateRouterChanged: hace falta leer la MAC guardada
// ANTES de que RecordScan la sobrescriba con la de este escaneo). key es
// un string plano (no history.NetworkKey) para no sumarle a este archivo
// un import que ningún otro de sus símbolos necesita; devices.go, en el
// mismo paquete, sí usa el tipo fuerte donde ya lo tenía a mano.
func (s *Store) NetworkByKey(ctx context.Context, key string) (Network, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+networkColumns+` FROM networks WHERE key = ?`, key)
	n, err := scanNetwork(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Network{}, ErrNotFound
	}
	return n, err
}

func scanNetwork(scan func(dest ...any) error) (Network, error) {
	var n Network
	var firstSeen, lastSeen string
	var lastScanAt *string
	if err := scan(&n.Key, &n.Subnet, &n.Gateway, &n.GatewayMAC, &n.Role, &firstSeen, &lastSeen, &lastScanAt); err != nil {
		return Network{}, err
	}
	var err error
	if n.FirstSeen, err = parseTime(firstSeen); err != nil {
		return Network{}, err
	}
	if n.LastSeen, err = parseTime(lastSeen); err != nil {
		return Network{}, err
	}
	if n.LastScanAt, err = parseTimePtr(lastScanAt); err != nil {
		return Network{}, err
	}
	return n, nil
}

// Stats son los números rápidos de "vigia-agent status": cuántas redes y
// equipos conoce la base, y cuándo terminó el último escaneo.
type Stats struct {
	Networks      int
	Devices       int
	OnlineDevices int
	LastScanAt    *time.Time
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM networks`).Scan(&st.Networks); err != nil {
		return Stats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices`).Scan(&st.Devices); err != nil {
		return Stats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE online = 1`).Scan(&st.OnlineDevices); err != nil {
		return Stats{}, err
	}
	var lastScanAt *string
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(last_scan_at) FROM networks`).Scan(&lastScanAt); err != nil {
		return Stats{}, err
	}
	var err error
	if st.LastScanAt, err = parseTimePtr(lastScanAt); err != nil {
		return Stats{}, err
	}
	return st, nil
}
