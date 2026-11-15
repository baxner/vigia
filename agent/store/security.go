package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/internal/ptrs"
	"github.com/baxner/vigia/core"
)

// Persistencia de "Seguridad (v1.2)" (ver docs/agente-api.md y
// docs/plan.md, Fase 5): el último RouterInfo que contestó
// UPnP/NAT-PMP, el historial de sus redirecciones (con first_seen/last_seen/
// active) y la memoria permanente de qué (equipo, puerto) ya disparó
// RISKY_SERVICE. Las reglas (qué cuenta como "nuevo", severidad, texto)
// viven en agent/alerts, puras; este archivo solo guarda y relee datos —
// mismo reparto que agent/history (reglas) y agent/store/devices.go (E/S).

// firstNonEmptyLocal: como alerts.firstNonEmpty, pero ese no está
// exportado y no vale la pena solo por este archivo.
func firstNonEmptyLocal(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// --- router_state: el último RouterInfo que contestó, por red. ---

// RouterState devuelve el último RouterInfo guardado de networkKey
// (enriquecido con sus redirecciones activas, ver ActivePortMappings), o
// nil si esa red nunca contestó UPnP ni NAT-PMP (ver GET /api/v1/security:
// "router: null si el router no contesta"). A propósito NO consulta el
// router en vivo: sirve lo último que se guardó en el ciclo periódico (ver
// docs/agente-api.md: "calculado al pedirlo con lo último de cada
// escaneo").
func (s *Store) RouterState(ctx context.Context, networkKey string) (*core.RouterInfo, error) {
	var ip string
	var mac, externalIP, reach *string
	var upnp, natpmp bool
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT ip, mac, upnp, natpmp, external_ip, reach, updated_at
		FROM router_state WHERE network_key = ?
	`, networkKey).Scan(&ip, &mac, &upnp, &natpmp, &externalIP, &reach, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	mappings, err := s.ActivePortMappings(ctx, networkKey)
	if err != nil {
		return nil, err
	}
	return &core.RouterInfo{
		IP:           ip,
		MAC:          ptrs.Deref(mac),
		UPnP:         upnp,
		NATPMP:       natpmp,
		ExternalIP:   ptrs.Deref(externalIP),
		Reach:        ptrs.Deref(reach),
		PortMappings: mappings,
	}, nil
}

// SaveRouterState reemplaza el RouterInfo guardado de networkKey (una fila
// por red: ver migration3). No toca port_mappings — eso lo hace
// ReconcilePortMappings aparte, con su propia lógica de línea
// base/reaparición.
func (s *Store) SaveRouterState(ctx context.Context, networkKey string, info *core.RouterInfo, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO router_state (network_key, ip, mac, upnp, natpmp, external_ip, reach, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(network_key) DO UPDATE SET
			ip = excluded.ip, mac = excluded.mac, upnp = excluded.upnp, natpmp = excluded.natpmp,
			external_ip = excluded.external_ip, reach = excluded.reach, updated_at = excluded.updated_at
	`, networkKey, info.IP, nilIfEmpty(info.MAC), info.UPnP, info.NATPMP,
		nilIfEmpty(info.ExternalIP), nilIfEmpty(info.Reach), formatTime(at))
	return err
}

// --- port_mappings: historial de redirecciones del router. ---

// ActivePortMappings devuelve las redirecciones activas de networkKey (ver
// RouterInfo.PortMappings), con DeviceID/FirstSeen/LastSeen ya resueltos
// desde la base — a diferencia de lo que devuelve core.QueryRouter en
// crudo (ver su comentario: esos tres quedan en cero-valor ahí). Orden por
// puerto exterior, solo por determinismo en las pruebas.
func (s *Store) ActivePortMappings(ctx context.Context, networkKey string) ([]core.PortMapping, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT protocol, external_port, internal_ip, internal_port, description, source, device_id, first_seen, last_seen
		FROM port_mappings WHERE network_key = ? AND active = 1
		ORDER BY external_port, internal_ip
	`, networkKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.PortMapping
	for rows.Next() {
		var pm core.PortMapping
		var description *string
		var firstSeen, lastSeen string
		if err := rows.Scan(&pm.Protocol, &pm.ExternalPort, &pm.InternalIP, &pm.InternalPort,
			&description, &pm.Source, &pm.DeviceID, &firstSeen, &lastSeen); err != nil {
			return nil, err
		}
		pm.Description = ptrs.Deref(description)
		fs, err := parseTime(firstSeen)
		if err != nil {
			return nil, err
		}
		ls, err := parseTime(lastSeen)
		if err != nil {
			return nil, err
		}
		pm.FirstSeen, pm.LastSeen = &fs, &ls
		out = append(out, pm)
	}
	return out, rows.Err()
}

// PortMappingNotifyKeysSeen: las PortMappingNotifyKey (equipo + programa,
// ver agent/alerts.EvaluateNewPortMappings) que alguna vez pidieron una
// redirección en esta red, activa o no — derivadas de TODO lo que ya está
// en port_mappings, incluida la descripción (ver docs/plan.md, Fase 5.1:
// "la memoria de 'ya visto' debe tomar en cuenta lo que ya está en la
// tabla port_mappings", para no avisar de nuevo, al actualizar, lo que ya
// se vio con el esquema de antes de esta fase).
func (s *Store) PortMappingNotifyKeysSeen(ctx context.Context, networkKey string) (map[alerts.PortMappingNotifyKey]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT protocol, internal_ip, internal_port, description FROM port_mappings WHERE network_key = ?
	`, networkKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := make(map[alerts.PortMappingNotifyKey]bool)
	for rows.Next() {
		var protocol, internalIP string
		var internalPort int
		var description *string
		if err := rows.Scan(&protocol, &internalIP, &internalPort, &description); err != nil {
			return nil, err
		}
		pm := core.PortMapping{Protocol: protocol, InternalIP: internalIP, InternalPort: internalPort, Description: ptrs.Deref(description)}
		seen[alerts.PortMappingNotifyKeyOf(pm)] = true
	}
	return seen, rows.Err()
}

// ReconcilePortMappings persiste `current` (lo que acaba de contestar el
// router) como el nuevo conjunto ACTIVO de redirecciones de networkKey:
// inserta las que no existían, reactiva (active=1, last_seen=at) las que
// reaparecen sin tocar su first_seen original, actualiza last_seen de las
// que ya estaban activas, y marca active=0 las que ya no están en
// `current`. PURO en el sentido de "no decide nada": qué es "nuevo" para
// efectos de PORT_MAPPING_ADDED lo decide agent/alerts.EvaluateNewPortMappings
// ANTES de llamar aquí (con el ActivePortMappings de ANTES de esta
// llamada); esta función solo refleja la realidad actual en la base.
func (s *Store) ReconcilePortMappings(ctx context.Context, networkKey string, current []core.PortMapping, at time.Time) error {
	_, err := withTx(ctx, s.db, func(tx *sql.Tx) (struct{}, error) {
		zero := struct{}{}
		rows, err := tx.QueryContext(ctx, `
			SELECT id, protocol, external_port, internal_ip FROM port_mappings
			WHERE network_key = ? AND active = 1
		`, networkKey)
		if err != nil {
			return zero, err
		}
		type activeRow struct {
			id  int64
			key alerts.PortMappingKey
		}
		var existing []activeRow
		for rows.Next() {
			var r activeRow
			if err := rows.Scan(&r.id, &r.key.Protocol, &r.key.ExternalPort, &r.key.InternalIP); err != nil {
				rows.Close()
				return zero, err
			}
			existing = append(existing, r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return zero, err
		}
		rows.Close()

		atStr := formatTime(at)
		currentSet := make(map[alerts.PortMappingKey]bool, len(current))
		for _, pm := range current {
			k := alerts.PortMappingKeyOf(pm)
			currentSet[k] = true
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO port_mappings (
					network_key, protocol, external_port, internal_ip, internal_port,
					description, source, device_id, first_seen, last_seen, active
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
				ON CONFLICT(network_key, protocol, external_port, internal_ip) DO UPDATE SET
					internal_port = excluded.internal_port,
					description   = excluded.description,
					source        = excluded.source,
					device_id     = excluded.device_id,
					last_seen     = excluded.last_seen,
					active        = 1
			`, networkKey, k.Protocol, k.ExternalPort, k.InternalIP, pm.InternalPort,
				nilIfEmpty(pm.Description), firstNonEmptyLocal(pm.Source, "upnp"), pm.DeviceID, atStr, atStr); err != nil {
				return zero, err
			}
		}

		for _, r := range existing {
			if currentSet[r.key] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE port_mappings SET active = 0 WHERE id = ?`, r.id); err != nil {
				return zero, err
			}
		}
		return zero, nil
	})
	return err
}

// --- security_baseline: evita la tormenta de alertas del primer arranque
// tras actualizar (ver agent/alerts.EvaluateNewPortMappings/
// EvaluateNewRiskyServices y el aviso "LÍNEA BASE" en
// docs/plan.md, Fase 5). ---

func (s *Store) PortMappingsBaselineDone(ctx context.Context) (bool, error) {
	var done bool
	err := s.db.QueryRowContext(ctx, `SELECT port_mappings_done FROM security_baseline WHERE id = 1`).Scan(&done)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // no debería pasar (migration3 inserta la fila): más vale tratarlo como "sin línea base" que reventar
	}
	return done, err
}

func (s *Store) SetPortMappingsBaselineDone(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE security_baseline SET port_mappings_done = 1 WHERE id = 1`)
	return err
}

func (s *Store) RiskyServicesBaselineDone(ctx context.Context) (bool, error) {
	var done bool
	err := s.db.QueryRowContext(ctx, `SELECT risky_services_done FROM security_baseline WHERE id = 1`).Scan(&done)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return done, err
}

func (s *Store) SetRiskyServicesBaselineDone(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE security_baseline SET risky_services_done = 1 WHERE id = 1`)
	return err
}

// --- risky_service_seen: memoria PERMANENTE de qué (equipo, puerto) ya
// disparó RISKY_SERVICE alguna vez (nunca se borra: ver
// agent/alerts.EvaluateNewRiskyServices). ---

// SeenRiskyPorts devuelve, para cada equipo de networkKey, el conjunto de
// puertos que ya dispararon RISKY_SERVICE alguna vez (deviceId -> puerto ->
// true). El join por devices.network_key es necesario porque
// risky_service_seen no guarda la red directamente (un equipo no cambia de
// red sin cambiar de id: ver history.Reconcile).
func (s *Store) SeenRiskyPorts(ctx context.Context, networkKey string) (map[int64]map[int]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rss.device_id, rss.port
		FROM risky_service_seen rss
		JOIN devices d ON d.id = rss.device_id
		WHERE d.network_key = ?
	`, networkKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]map[int]bool)
	for rows.Next() {
		var deviceID int64
		var port int
		if err := rows.Scan(&deviceID, &port); err != nil {
			return nil, err
		}
		if out[deviceID] == nil {
			out[deviceID] = make(map[int]bool)
		}
		out[deviceID][port] = true
	}
	return out, rows.Err()
}

// RecordRiskyPortSeen marca (deviceID, port) como ya alertado, para
// siempre: ON CONFLICT DO NOTHING porque reafirmar un par ya visto no debe
// fallar ni actualizar nada (first_seen es la primera vez de verdad, no la
// más reciente).
func (s *Store) RecordRiskyPortSeen(ctx context.Context, deviceID int64, port int, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO risky_service_seen (device_id, port, first_seen) VALUES (?, ?, ?)
		ON CONFLICT(device_id, port) DO NOTHING
	`, deviceID, port, formatTime(at))
	return err
}

// --- important_offline_pending: qué equipos importantes tienen un
// IMPORTANT_OFFLINE sin su IMPORTANT_ONLINE todavía (Fase 5.1, ver
// docs/plan.md y agent/alerts.EvaluateImportantOffline/
// EvaluateImportantOnline). A diferencia de risky_service_seen, esto NO es
// permanente: se limpia en cuanto el equipo vuelve, para que el PRÓXIMO
// episodio pueda avisar de nuevo.

// ImportantOfflinePending devuelve, para cada equipo con un episodio
// pendiente, true (deviceId -> true). Nunca nil: mapa vacío si no hay
// ninguno, para que EvaluateImportantOnline no tenga que distinguir nil de
// vacío.
func (s *Store) ImportantOfflinePending(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device_id FROM important_offline_pending`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pending := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		pending[id] = true
	}
	return pending, rows.Err()
}

// SetImportantOfflinePending marca deviceID con un episodio pendiente. ON
// CONFLICT DO NOTHING: volver a marcarlo (el mismo episodio, en el
// siguiente escaneo) no es un error ni debe tocar nada.
func (s *Store) SetImportantOfflinePending(ctx context.Context, deviceID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO important_offline_pending (device_id) VALUES (?) ON CONFLICT(device_id) DO NOTHING
	`, deviceID)
	return err
}

// ClearImportantOfflinePending deshace el pendiente de deviceID (el
// episodio se resolvió, con o sin IMPORTANT_ONLINE avisado). No es error
// que no hubiera ninguno (idempotente, igual que el resto del contrato).
func (s *Store) ClearImportantOfflinePending(ctx context.Context, deviceID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM important_offline_pending WHERE device_id = ?`, deviceID)
	return err
}

// --- acknowledged_findings: "ya lo sé, es a propósito" (v1.3, ver
// docs/agente-api.md "Hallazgos reconocidos" y docs/plan.md, Fase 5.1).

// SetAcknowledged marca findingID como reconocido: upsert (ON CONFLICT
// DO UPDATE), para que repetir el PUT —p. ej. solo para cambiar la nota—
// no falle y reemplace at/note con los nuevos.
func (s *Store) SetAcknowledged(ctx context.Context, findingID string, note *string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO acknowledged_findings (finding_id, at, note) VALUES (?, ?, ?)
		ON CONFLICT(finding_id) DO UPDATE SET at = excluded.at, note = excluded.note
	`, findingID, formatTime(at), note)
	return err
}

// RemoveAcknowledged deshace el reconocimiento de findingID ("Deshacer",
// ver docs/agente-api.md). No es error que no estuviera reconocido.
func (s *Store) RemoveAcknowledged(ctx context.Context, findingID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM acknowledged_findings WHERE finding_id = ?`, findingID)
	return err
}

// Acknowledgements devuelve todos los reconocimientos vigentes (ver
// core.SecurityInput.Acknowledged, que BuildSecurityReport aplica por
// FindingID — no por posición ni por tipo). Nunca nil.
func (s *Store) Acknowledgements(ctx context.Context) ([]core.Acknowledgement, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT finding_id, at, note FROM acknowledged_findings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Acknowledgement{}
	for rows.Next() {
		var id, at string
		var note *string
		if err := rows.Scan(&id, &at, &note); err != nil {
			return nil, err
		}
		t, err := parseTime(at)
		if err != nil {
			return nil, err
		}
		out = append(out, core.Acknowledgement{FindingID: id, At: t, Note: note})
	}
	return out, rows.Err()
}
