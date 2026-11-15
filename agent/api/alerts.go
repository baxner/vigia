package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/store"
)

// handleAlerts sirve GET /api/v1/alerts?after=&limit= (ver
// docs/agente-api.md: "Alertas con id > after, de la más vieja a la más
// nueva, para ponerse al día").
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	after, ok := queryNonNegativeInt(r, "after", 0)
	if !ok {
		writeError(w, http.StatusBadRequest, "after inválido")
		return
	}
	limit, ok := queryNonNegativeInt(r, "limit", 0)
	if !ok {
		writeError(w, http.StatusBadRequest, "limit inválido")
		return
	}
	if limit > maxQueryLimit {
		limit = maxQueryLimit
	}
	out, err := s.store.Alerts(r.Context(), after, int(limit))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	if out == nil {
		out = []store.Alert{}
	}
	writeJSON(w, http.StatusOK, out)
}

// queryNonNegativeInt lee un parámetro de query opcional como entero no
// negativo; ausente devuelve def. ok=false si vino pero no es válido.
func queryNonNegativeInt(r *http.Request, name string, def int64) (int64, bool) {
	q := r.URL.Query().Get(name)
	if q == "" {
		return def, true
	}
	n, err := strconv.ParseInt(q, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// alertsReadRequest: cuerpo de POST /api/v1/alerts/read (ver
// docs/agente-api.md).
type alertsReadRequest struct {
	UpTo int64 `json:"upTo"`
}

func (s *Server) handleAlertsRead(w http.ResponseWriter, r *http.Request) {
	var req alertsReadRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	if err := s.store.MarkAlertsRead(r.Context(), req.UpTo); err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAlertSettingsGet(w http.ResponseWriter, r *http.Request) {
	set, err := s.store.AlertSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// handleAlertSettingsPut sirve PUT /api/v1/alert-settings: reemplazo
// completo (a diferencia de PATCH /api/v1/devices/{id}, aquí no hay
// "campo ausente = sin cambio" — un tipo que no venga en el cuerpo queda
// en false, como cualquier PUT).
func (s *Server) handleAlertSettingsPut(w http.ResponseWriter, r *http.Request) {
	var set alerts.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&set); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	if err := s.store.SetAlertSettings(r.Context(), set); err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusOK, set)
}
