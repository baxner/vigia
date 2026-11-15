package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/webhooks"
)

// webhookCreateRequest: cuerpo de POST /api/v1/webhooks (ver
// docs/agente-api.md).
type webhookCreateRequest struct {
	URL    string   `json:"url"`
	Types  []string `json:"types"`
	Secret string   `json:"secret"`
}

func (s *Server) handleWebhooksList(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.Webhooks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	if hooks == nil {
		hooks = []store.Webhook{}
	}
	writeJSON(w, http.StatusOK, hooks)
}

// handleWebhooksCreate sirve POST /api/v1/webhooks. Se valida el esquema
// de la URL una sola vez aquí ("Solo http/https" del contrato), no en
// cada entrega.
func (s *Server) handleWebhooksCreate(w http.ResponseWriter, r *http.Request) {
	var req webhookCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if err := webhooks.ValidateWebhookURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, "url inválida: "+err.Error())
		return
	}
	hook, err := s.store.CreateWebhook(r.Context(), req.URL, req.Types, req.Secret, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusCreated, hook)
}

func webhookPathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (s *Server) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := webhookPathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	if err := s.store.DeleteWebhook(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook no encontrado")
			return
		}
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWebhookTest sirve POST /api/v1/webhooks/{id}/test: UN solo
// intento (sin reintentos: es un botón de "probar ahora", no una entrega
// real) con la misma firma/cabeceras que una entrega de verdad, sin
// importar el filtro `types` del webhook — probarlo debe mandarle algo
// siempre, sea o no de un tipo al que está suscrito. El resultado se
// guarda igual que cualquier entrega (lastStatus/lastError).
func (s *Server) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	id, ok := webhookPathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id inválido")
		return
	}
	hook, err := s.store.WebhookByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "webhook no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}

	test := alerts.TestAlert(time.Now().UTC())
	body, err := json.Marshal(webhooks.Payload{
		Event: "alert",
		Alert: store.Alert{Type: test.Type, Severity: test.Severity, At: test.At, Title: test.Title, Body: test.Body},
		Agent: webhooks.AgentInfo{ID: s.identity.ID, Name: s.identity.Name},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	deliveryID, err := webhooks.NewDeliveryID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}

	target := webhooks.Target{ID: hook.ID, URL: hook.URL, Secret: hook.Secret}
	status, sendErr := s.sender.Attempt(r.Context(), target, body, string(test.Type), deliveryID)
	if sendErr != nil {
		msg := sanitizeWebhookSendError(sendErr)
		_ = s.store.UpdateWebhookResult(r.Context(), hook.ID, nil, &msg)
		writeJSON(w, http.StatusOK, map[string]string{"error": msg})
		return
	}
	_ = s.store.UpdateWebhookResult(r.Context(), hook.ID, &status, nil)
	writeJSON(w, http.StatusOK, map[string]int{"status": status})
}

func sanitizeWebhookSendError(err error) string {
	switch {
	case errors.Is(err, webhooks.ErrBlockedAddress):
		return "destino de webhook no permitido"
	case errors.Is(err, context.DeadlineExceeded):
		return "tiempo de espera agotado al contactar el webhook"
	case errors.Is(err, context.Canceled):
		return "solicitud cancelada"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "tiempo de espera agotado al contactar el webhook"
	}
	return "no se pudo conectar con el destino del webhook"
}
