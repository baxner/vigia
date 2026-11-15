package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/internal/ptrs"
	"github.com/baxner/vigia/core"
)

// ErrNotFound: el id o clave pedido no existe.
var ErrNotFound = errors.New("store: no encontrado")

const deviceColumns = `
	id, network_key, mac, last_ip, ssdp_udn, strong_name,
	first_seen, last_seen, online, online_since, offline_misses, last_json,
	custom_name, custom_type_id, custom_notes, custom_location, custom_important
`

// deviceRow: un KnownDevice más su offlineMisses (contabilidad interna de
// history.Missing, no forma parte del contrato — ver migration1).
type deviceRow struct {
	device        history.KnownDevice
	offlineMisses int
}

func scanDevice(scan func(dest ...any) error) (deviceRow, error) {
	var (
		d                                                     history.KnownDevice
		networkKey                                            string
		ssdpUDN, strongName                                   *string // indexables, no se exponen: se recalculan desde last_json
		firstSeen, lastSeen                                   string
		onlineSince                                           *string
		offlineMisses                                         int
		lastJSON                                              string
		customName, customTypeID, customNotes, customLocation *string
		customImportant                                       bool
	)
	if err := scan(&d.ID, &networkKey, &d.MAC, &d.LastIP, &ssdpUDN, &strongName,
		&firstSeen, &lastSeen, &d.Online, &onlineSince, &offlineMisses, &lastJSON,
		&customName, &customTypeID, &customNotes, &customLocation, &customImportant); err != nil {
		return deviceRow{}, err
	}

	d.Network = history.NetworkKey(networkKey)
	var err error
	if d.FirstSeen, err = parseTime(firstSeen); err != nil {
		return deviceRow{}, fmt.Errorf("first_seen: %w", err)
	}
	if d.LastSeen, err = parseTime(lastSeen); err != nil {
		return deviceRow{}, fmt.Errorf("last_seen: %w", err)
	}
	if d.OnlineSince, err = parseTimePtr(onlineSince); err != nil {
		return deviceRow{}, fmt.Errorf("online_since: %w", err)
	}
	var last core.Device
	if err := json.Unmarshal([]byte(lastJSON), &last); err != nil {
		return deviceRow{}, fmt.Errorf("last_json: %w", err)
	}
	d.Last = last
	d.Custom = history.DeviceCustomization{
		Name: customName, TypeID: customTypeID, Notes: customNotes, Location: customLocation,
		Important: customImportant,
	}
	return deviceRow{device: d, offlineMisses: offlineMisses}, nil
}

func deviceByID(ctx context.Context, q querier, id int64) (deviceRow, error) {
	row := q.QueryRowContext(ctx, "SELECT "+deviceColumns+" FROM devices WHERE id = ?", id)
	r, err := scanDevice(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return deviceRow{}, ErrNotFound
	}
	return r, err
}

func devicesByNetwork(ctx context.Context, q querier, network history.NetworkKey) ([]deviceRow, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+deviceColumns+" FROM devices WHERE network_key = ?", string(network))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []deviceRow
	for rows.Next() {
		r, err := scanDevice(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// nilIfEmpty: "" se guarda como NULL (MAC/UDN/nombre sin resolver), no como
// cadena vacía — más limpio para consultar e idéntico a como lo ve la app
// (String? en vez de "").
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// indexableUDN/indexableStrongName: columnas indexables recalculadas desde
// el Device del escaneo, igual que DeviceEntity.firstSeen/mergedWith en
// Kotlin (ver migration1).
func indexableUDN(d core.Device) *string {
	return nilIfEmpty(strings.TrimSpace(d.Extra[history.ExtraSSDPUDN]))
}
func indexableStrongName(d core.Device) *string {
	return nilIfEmpty(history.StrongNameOf(d))
}

// ipSortKey: clave para ordenar IPv4 numéricamente ("192.168.0.9" antes que
// "192.168.0.29", no al revés como saldría en texto). Puerto de ipSortKey
// en RoomHistoryRepository.kt; una IP rara (IPv6, vacía, inválida) no
// revienta el orden: va al final.
func ipSortKey(ip string) uint64 {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return math.MaxUint64
	}
	var key uint64
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return math.MaxUint64
		}
		key = key<<8 | uint64(n)
	}
	return key
}

// DeviceByID devuelve el equipo id, o ErrNotFound si no existe.
func (s *Store) DeviceByID(ctx context.Context, id int64) (history.KnownDevice, error) {
	r, err := deviceByID(ctx, s.db, id)
	return r.device, err
}

// DevicesByNetwork devuelve los equipos de network (conectados y sin
// conexión), ordenados por IP numérica.
func (s *Store) DevicesByNetwork(ctx context.Context, network history.NetworkKey) ([]history.KnownDevice, error) {
	rows, err := devicesByNetwork(ctx, s.db, network)
	if err != nil {
		return nil, err
	}
	out := make([]history.KnownDevice, len(rows))
	for i, r := range rows {
		out[i] = r.device
	}
	sort.SliceStable(out, func(i, j int) bool { return ipSortKey(out[i].LastIP) < ipSortKey(out[j].LastIP) })
	return out, nil
}

// NetworkInfo identifica la red de un escaneo (ver history.NewNetworkKey):
// Key = "<gateway>|<subred>". GatewayMAC es "" si no se pudo resolver.
type NetworkInfo struct {
	Key        history.NetworkKey
	Subnet     string
	Gateway    string
	GatewayMAC string
	// Role (v1.2, ver docs/agente-api.md, "Seguridad" > Anillos): "inner" |
	// "outer". "" (el valor que ya mandaban todos los llamadores de antes
	// de la Fase 5) se trata como "inner" — ver defaultRole en networks.go.
	Role string
}

// RecordedEvent pareja un evento nuevo con el KnownDevice al que
// corresponde, ya actualizado. Los tags JSON son el contrato tal cual: el
// paquete api la sirve directo como el "data" del evento SSE "device" (ver
// docs/agente-api.md, sección Stream).
type RecordedEvent struct {
	Event  history.DeviceEvent `json:"event"`
	Device history.KnownDevice `json:"device"`
}

// RecordScan registra un escaneo completo de net: reconcilia cada
// core.Device con los equipos ya conocidos de esa red (history.Reconcile),
// funde estado y decide eventos (history.Seen/Missing) y persiste todo en
// una sola transacción. Devuelve los eventos nuevos en el orden en que
// surgen de recorrer primero los escaneados (altas y reconexiones/cambios
// de IP) y luego los conocidos que faltaron (mismo orden que
// RoomHistoryRepository.recordScan en Kotlin).
func (s *Store) RecordScan(ctx context.Context, net NetworkInfo, devices []core.Device, at time.Time) ([]RecordedEvent, error) {
	return withTx(ctx, s.db, func(tx *sql.Tx) ([]RecordedEvent, error) {
		if err := touchNetwork(ctx, tx, string(net.Key), net.Subnet, net.Gateway, nilIfEmpty(net.GatewayMAC), net.Role, at); err != nil {
			return nil, fmt.Errorf("red: %w", err)
		}

		known, err := devicesByNetwork(ctx, tx, net.Key)
		if err != nil {
			return nil, fmt.Errorf("leer equipos: %w", err)
		}
		knownDevices := make([]history.KnownDevice, len(known))
		offlineMisses := make(map[int64]int, len(known))
		for i, r := range known {
			knownDevices[i] = r.device
			offlineMisses[r.device.ID] = r.offlineMisses
		}

		matches := history.Reconcile(knownDevices, devices)
		matched := make(map[int64]bool, len(matches))
		var out []RecordedEvent

		for _, m := range matches {
			if m.Known == nil {
				id, err := insertFirstSeen(ctx, tx, net.Key, m.Scanned, at)
				if err != nil {
					return nil, fmt.Errorf("alta: %w", err)
				}
				ev := history.DeviceEvent{DeviceID: id, Type: history.FirstSeen, At: at}
				if _, err := insertEvent(ctx, tx, ev); err != nil {
					return nil, fmt.Errorf("evento de alta: %w", err)
				}
				out = append(out, RecordedEvent{Event: ev, Device: newKnownDevice(id, net.Key, m.Scanned, at)})
				continue
			}

			matched[m.Known.ID] = true
			outcome := history.Seen(*m.Known, m.Scanned, at)
			if err := updateDevice(ctx, tx, outcome.Device, outcome.OfflineMisses); err != nil {
				return nil, fmt.Errorf("actualizar equipo %d: %w", m.Known.ID, err)
			}
			for _, ev := range outcome.Events {
				if _, err := insertEvent(ctx, tx, ev); err != nil {
					return nil, fmt.Errorf("evento: %w", err)
				}
				out = append(out, RecordedEvent{Event: ev, Device: outcome.Device})
			}
		}

		for _, r := range known {
			if matched[r.device.ID] {
				continue
			}
			outcome := history.Missing(r.device, offlineMisses[r.device.ID], at)
			if err := updateDevice(ctx, tx, outcome.Device, outcome.OfflineMisses); err != nil {
				return nil, fmt.Errorf("actualizar equipo %d: %w", r.device.ID, err)
			}
			for _, ev := range outcome.Events {
				if _, err := insertEvent(ctx, tx, ev); err != nil {
					return nil, fmt.Errorf("evento: %w", err)
				}
				out = append(out, RecordedEvent{Event: ev, Device: outcome.Device})
			}
		}

		if err := markScanned(ctx, tx, string(net.Key), at); err != nil {
			return nil, fmt.Errorf("marcar escaneo: %w", err)
		}

		return out, nil
	})
}

// newKnownDevice: la fila que corresponde a un equipo visto por primera
// vez. Puerto de DeviceEntity.firstSeen + toDomain en Kotlin.
func newKnownDevice(id int64, network history.NetworkKey, scanned core.Device, at time.Time) history.KnownDevice {
	return history.KnownDevice{
		ID:          id,
		Network:     network,
		MAC:         nilIfEmpty(scanned.MAC),
		LastIP:      scanned.IP,
		FirstSeen:   at,
		LastSeen:    at,
		Online:      true,
		OnlineSince: ptrs.To(at),
		Last:        scanned,
	}
}

func insertFirstSeen(ctx context.Context, tx *sql.Tx, network history.NetworkKey, scanned core.Device, at time.Time) (int64, error) {
	lastJSON, err := json.Marshal(scanned)
	if err != nil {
		return 0, err
	}
	atStr := formatTime(at)
	res, err := tx.ExecContext(ctx, `
		INSERT INTO devices (
			network_key, mac, last_ip, ssdp_udn, strong_name,
			first_seen, last_seen, online, online_since, offline_misses, last_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, 0, ?)
	`, string(network), nilIfEmpty(scanned.MAC), scanned.IP, indexableUDN(scanned), indexableStrongName(scanned),
		atStr, atStr, atStr, string(lastJSON))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// updateDevice reescribe la fila id con el KnownDevice ya fusionado (ver
// history.Seen/Missing) y su offlineMisses. Puerto de
// DeviceEntity.mergedWith: también reescribe custom_* con los valores que ya
// traía d.Custom (history.Seen/Missing nunca los toca), así que no hace
// falta una ruta aparte que los deje intactos.
func updateDevice(ctx context.Context, tx *sql.Tx, d history.KnownDevice, offlineMisses int) error {
	lastJSON, err := json.Marshal(d.Last)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE devices SET
			mac = ?, last_ip = ?, ssdp_udn = ?, strong_name = ?,
			last_seen = ?, online = ?, online_since = ?, offline_misses = ?, last_json = ?,
			custom_name = ?, custom_type_id = ?, custom_notes = ?, custom_location = ?, custom_important = ?
		WHERE id = ?
	`, d.MAC, d.LastIP, indexableUDN(d.Last), indexableStrongName(d.Last),
		formatTime(d.LastSeen), d.Online, formatTimePtr(d.OnlineSince), offlineMisses, string(lastJSON),
		d.Custom.Name, d.Custom.TypeID, d.Custom.Notes, d.Custom.Location, d.Custom.Important,
		d.ID)
	return err
}

// CustomizationPatch es un PATCH parcial de DeviceCustomization: cada par
// Set*/valor viene de si la clave llegó en el cuerpo JSON (ver "PATCH
// /api/v1/devices/{id}" en docs/agente-api.md: ausente = no tocar; dentro de
// lo que llegó, puntero nil = borrar). Lo construye el paquete api al
// parsear el cuerpo; este paquete solo lo aplica.
type CustomizationPatch struct {
	Name        *string
	SetName     bool
	TypeID      *string
	SetTypeID   bool
	Notes       *string
	SetNotes    bool
	Location    *string
	SetLocation bool
	// Important no admite "borrar" (no es nullable en el contrato): solo
	// ausente (SetImportant=false) o un valor concreto.
	Important    bool
	SetImportant bool
}

// UpdateCustomization aplica patch sobre el equipo id y devuelve el
// KnownDevice resultante. ErrNotFound si no existe.
func (s *Store) UpdateCustomization(ctx context.Context, id int64, patch CustomizationPatch) (history.KnownDevice, error) {
	return withTx(ctx, s.db, func(tx *sql.Tx) (history.KnownDevice, error) {
		r, err := deviceByID(ctx, tx, id)
		if err != nil {
			return history.KnownDevice{}, err
		}
		custom := r.device.Custom
		if patch.SetName {
			custom.Name = patch.Name
		}
		if patch.SetTypeID {
			custom.TypeID = patch.TypeID
		}
		if patch.SetNotes {
			custom.Notes = patch.Notes
		}
		if patch.SetLocation {
			custom.Location = patch.Location
		}
		if patch.SetImportant {
			custom.Important = patch.Important
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE devices SET custom_name = ?, custom_type_id = ?, custom_notes = ?, custom_location = ?, custom_important = ?
			WHERE id = ?
		`, custom.Name, custom.TypeID, custom.Notes, custom.Location, custom.Important, id)
		if err != nil {
			return history.KnownDevice{}, err
		}
		r.device.Custom = custom
		return r.device, nil
	})
}

// Forget olvida el equipo id y su cronología (ON DELETE CASCADE se encarga
// de device_events). ErrNotFound si no existe.
func (s *Store) Forget(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM devices WHERE id = ?", id)
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
