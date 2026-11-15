package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/core"
)

// handleSecurity sirve GET /api/v1/security (ver docs/agente-api.md,
// "Seguridad v1.2"): arma el core.SecurityInput enteramente a partir de lo
// que YA está guardado (el escaneo periódico del anillo interior, la
// última consulta al router, el último escaneo del anillo exterior, las
// alertas de las últimas 24 h) y llama a core.BuildSecurityReport — nunca
// dispara un escaneo ni una consulta al router en vivo (ver el contrato:
// "calculado al pedirlo con lo último de cada escaneo"). Sin router ni
// escaneo todavía, el informe igual sale: BuildSecurityReport tolera
// Router/OuterRing nil y Devices/RecentAlerts vacíos.
func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	nets, err := s.store.Networks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	inner := latestNetwork(nets, "inner")
	outerNet := latestNetwork(nets, "outer")

	var devices []core.SecurityDevice
	var router *core.RouterInfo
	if inner != nil {
		known, err := s.store.DevicesByNetwork(ctx, history.NetworkKey(inner.Key))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error interno")
			return
		}
		devices = toSecurityDevices(known)

		router, err = s.store.RouterState(ctx, inner.Key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error interno")
			return
		}
	}

	var outerRing *core.OuterRing
	if outerNet != nil {
		known, err := s.store.DevicesByNetwork(ctx, history.NetworkKey(outerNet.Key))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error interno")
			return
		}
		outerRing = &core.OuterRing{
			Network:    outerNet.Key,
			Subnet:     outerNet.Subnet,
			Gateway:    outerNet.Gateway,
			LastScanAt: outerNet.LastScanAt,
			Devices:    toSecurityDevices(known),
		}
	}

	since := time.Now().UTC().Add(-24 * time.Hour)
	stored, err := s.store.RecentAlerts(ctx, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	recentAlerts := make([]core.Alert, len(stored))
	for i, a := range stored {
		recentAlerts[i] = core.Alert{Type: string(a.Type), At: a.At}
	}

	acks, err := s.store.Acknowledgements(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}

	report := core.BuildSecurityReport(core.SecurityInput{
		Devices:      devices,
		Router:       router,
		OuterRing:    outerRing,
		Wifi:         nil, // el agente corre en un host fijo sin inspeccionar Wi-Fi (ver docs/agente-api.md)
		RecentAlerts: recentAlerts,
		Acknowledged: acks,
	}, time.Now().UTC())
	writeJSON(w, http.StatusOK, report)
}

// acknowledgedBody: cuerpo opcional de PUT /api/v1/security/acknowledged/{findingId}
// (ver docs/agente-api.md, "Hallazgos reconocidos v1.3").
type acknowledgedBody struct {
	Note *string `json:"note"`
}

// parseAcknowledgedPath extrae {findingId} de PUT/DELETE
// /api/v1/security/acknowledged/{findingId}. {findingId} va URL-encoded
// (ver el contrato) y puede traer "/" de verdad (p. ej.
// "PORT_MAPPING|dev:12|7070/TCP"): igual que parseNetworkDevicesPath en
// handlers.go, hay que separar el prefijo sobre EscapedPath() (sin
// decodificar) y recién ahí hacer un único url.PathUnescape — un patrón
// "{findingId}" del mux de la stdlib se rompería apenas %2F se decodifique
// a "/" en r.URL.Path.
func parseAcknowledgedPath(escapedPath string) (findingID string, ok bool) {
	const prefix = "/api/v1/security/acknowledged/"
	if !strings.HasPrefix(escapedPath, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(escapedPath, prefix)
	if rest == "" {
		return "", false
	}
	decoded, err := url.PathUnescape(rest)
	if err != nil || decoded == "" {
		return "", false
	}
	return decoded, true
}

// handleAcknowledgedPut sirve PUT /api/v1/security/acknowledged/{findingId}
// ("ya lo sé, es a propósito"): upsert, cuerpo opcional {"note": "…"}.
func (s *Server) handleAcknowledgedPut(w http.ResponseWriter, r *http.Request) {
	findingID, ok := parseAcknowledgedPath(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}
	var body acknowledgedBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	if err := s.store.SetAcknowledged(r.Context(), findingID, body.Note, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAcknowledgedDelete sirve DELETE /api/v1/security/acknowledged/{findingId}
// ("Deshacer"). No es error que findingId no estuviera reconocido.
func (s *Server) handleAcknowledgedDelete(w http.ResponseWriter, r *http.Request) {
	findingID, ok := parseAcknowledgedPath(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.store.RemoveAcknowledged(r.Context(), findingID); err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// latestNetwork elige, entre nets, la de role `role` con el lastScanAt (o,
// a falta de eso, lastSeen) más reciente — "la que se está vigilando
// ahora", por si quedaron redes viejas abandonadas (router reemplazado,
// subred reasignada) con la misma role. role "inner" también acepta ""
// (redes de antes de la Fase 5, o escritas por un llamador que no fijó
// Role: ver defaultRole en agent/store). nil si no hay ninguna.
func latestNetwork(nets []store.Network, role string) *store.Network {
	var best *store.Network
	for i := range nets {
		n := &nets[i]
		r := n.Role
		if r == "" {
			r = "inner"
		}
		if r != role {
			continue
		}
		if best == nil || networkRecency(*n).After(networkRecency(*best)) {
			best = n
		}
	}
	return best
}

func networkRecency(n store.Network) time.Time {
	if n.LastScanAt != nil {
		return *n.LastScanAt
	}
	return n.LastSeen
}

// toSecurityDevices convierte []history.KnownDevice en []core.SecurityDevice
// (ver docs/agente-api.md: SecurityDevice es la forma corta que necesita
// el motor, con el nombre ya resuelto). nunca nil: [] si known está vacío,
// para que el JSON sea "[]" y no "null" (mismo criterio que el resto de
// los endpoints que sirven listas).
func toSecurityDevices(known []history.KnownDevice) []core.SecurityDevice {
	out := make([]core.SecurityDevice, len(known))
	for i, d := range known {
		id := d.ID
		out[i] = core.SecurityDevice{ID: &id, Name: alerts.DisplayName(d), Online: d.Online, Device: d.Last}
	}
	return out
}
