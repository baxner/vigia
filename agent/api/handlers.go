package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/store"
)

// infoResponse: GET /api/v1/info (sin token, ver docs/agente-api.md).
type infoResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Version         string  `json:"version"`
	ScanIntervalSec int     `json:"scanIntervalSec"`
	LastScanAt      *string `json:"lastScanAt"`
	// Warning: aviso de salud del último escaneo (p. ej. sin permiso de Red
	// local en macOS); ausente si todo va bien.
	Warning string `json:"warning,omitempty"`
	// Addresses: IPv4 de las interfaces activas (incluida la de
	// Tailscale, 100.x), para que la app alcance al agente fuera de casa
	// (v1.1, ver docs/agente-api.md). Nunca null: [] si no hay ninguna.
	Addresses []string `json:"addresses"`
	// CertSHA256: huella SHA-256 (hex) del certificado TLS del agente (ver
	// TLS v1.5, docs/agente-api.md). La app la guarda al vincular (o, para
	// un vínculo de antes de esta fase, la primera vez que lee /info por la
	// vía vieja dentro de la LAN) y desde ahí la fija: si el certificado
	// cambia, se niega a conectar.
	CertSHA256 string `json:"certSha256"`
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	resp := infoResponse{
		ID:         s.identity.ID,
		Name:       s.identity.Name,
		Version:    s.version,
		Addresses:  s.addresses(),
		CertSHA256: s.certSHA256,
	}
	if resp.Addresses == nil {
		resp.Addresses = []string{}
	}
	if s.warning != nil {
		resp.Warning = s.warning()
	}
	if s.scheduler != nil {
		resp.ScanIntervalSec = int(s.scheduler.Interval() / time.Second)
		if last := s.scheduler.LastScanAt(); !last.IsZero() {
			formatted := last.UTC().Format(time.RFC3339Nano)
			resp.LastScanAt = &formatted
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleNetworks(w http.ResponseWriter, r *http.Request) {
	nets, err := s.store.Networks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	if nets == nil {
		nets = []store.Network{}
	}
	writeJSON(w, http.StatusOK, nets)
}

// handleNetworkDevices sirve GET /api/v1/networks/{key}/devices. {key} se
// extrae a mano de EscapedPath() — ver el comentario en server.go.
func (s *Server) handleNetworkDevices(w http.ResponseWriter, r *http.Request) {
	key, ok := parseNetworkDevicesPath(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}
	devices, err := s.store.DevicesByNetwork(r.Context(), history.NetworkKey(key))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	// Red desconocida o sin equipos: igual 200 con lista vacía (el
	// contrato no pide 404 para esto, a diferencia de un id de equipo que
	// no existe).
	if devices == nil {
		devices = []history.KnownDevice{}
	}
	writeJSON(w, http.StatusOK, devices)
}

func parseNetworkDevicesPath(escapedPath string) (key string, ok bool) {
	const prefix = "/api/v1/networks/"
	const suffix = "/devices"
	if !strings.HasPrefix(escapedPath, prefix) || !strings.HasSuffix(escapedPath, suffix) {
		return "", false
	}
	middle := strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix)
	if middle == "" {
		return "", false
	}
	decoded, err := url.PathUnescape(middle)
	if err != nil || decoded == "" {
		return "", false
	}
	return decoded, true
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	d, err := s.store.DeviceByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "equipo no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeviceEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}

	limit := 0
	if q := r.URL.Query().Get("limit"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "limit inválido")
			return
		}
		if n > maxQueryLimit {
			n = maxQueryLimit
		}
		limit = n
	}

	// Events no distingue "id inexistente" de "sin eventos todavía"; lo
	// verificamos aparte para poder devolver 404 cuando corresponde.
	if _, err := s.store.DeviceByID(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "equipo no encontrado")
			return
		}
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}

	events, err := s.store.Events(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	if events == nil {
		events = []history.DeviceEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handlePatchDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}

	patch := store.CustomizationPatch{}
	if v, present := raw["name"]; present {
		val, err := decodeNullableString(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "name inválido")
			return
		}
		patch.SetName, patch.Name = true, val
	}
	if v, present := raw["typeId"]; present {
		val, err := decodeNullableString(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "typeId inválido")
			return
		}
		patch.SetTypeID, patch.TypeID = true, val
	}
	if v, present := raw["notes"]; present {
		val, err := decodeNullableString(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "notes inválido")
			return
		}
		patch.SetNotes, patch.Notes = true, val
	}
	if v, present := raw["location"]; present {
		val, err := decodeNullableString(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "location inválido")
			return
		}
		patch.SetLocation, patch.Location = true, val
	}
	if v, present := raw["important"]; present {
		// json.Unmarshal(null, &b) no da error (null-a-cualquier-cosa-que-no-
		// sea-puntero/mapa/slice/interface es, a propósito, un no-op según la
		// documentación de encoding/json) — hay que rechazar "null" a mano.
		if string(v) == "null" {
			writeError(w, http.StatusBadRequest, "important inválido: no admite null")
			return
		}
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			writeError(w, http.StatusBadRequest, "important inválido")
			return
		}
		patch.SetImportant, patch.Important = true, b
	}

	updated, err := s.store.UpdateCustomization(r.Context(), id, patch)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "equipo no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	if err := s.store.Forget(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "equipo no encontrado")
			return
		}
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil || !s.scheduler.TriggerNow() {
		writeError(w, http.StatusConflict, "ya hay un escaneo en curso")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}
