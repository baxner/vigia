package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// pingInterval: cada cuánto se manda ": ping" para mantener viva la
// conexión SSE (ver docs/agente-api.md, sección Stream).
const pingInterval = 25 * time.Second

// Hub difunde eventos "scan" y "device" a quienes están escuchando GET
// /api/v1/stream. Publish nunca bloquea: un suscriptor lento se queda sin
// ese evento (su buffer se descarta) en vez de frenar a los demás o al
// escaneo que está publicando.
type Hub struct {
	mu   sync.Mutex
	subs map[chan sseMessage]struct{}
}

type sseMessage struct {
	event string
	data  []byte
}

// NewHub crea un Hub vacío.
func NewHub() *Hub { return &Hub{subs: make(map[chan sseMessage]struct{})} }

func (h *Hub) subscribe() chan sseMessage {
	ch := make(chan sseMessage, 128)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan sseMessage) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *Hub) publish(event string, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- sseMessage{event: event, data: data}:
		default: // suscriptor lento: se descarta este evento para él, no se bloquea publish
		}
	}
}

// scanPayload: "data" del evento SSE "scan" (ver docs/agente-api.md).
type scanPayload struct {
	Network    string    `json:"network"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Devices    int       `json:"devices"`
}

// PublishScan difunde el evento "scan" de un escaneo que acaba de
// terminar.
func (h *Hub) PublishScan(network string, startedAt, finishedAt time.Time, devices int) {
	data, err := json.Marshal(scanPayload{Network: network, StartedAt: startedAt, FinishedAt: finishedAt, Devices: devices})
	if err != nil {
		return
	}
	h.publish("scan", data)
}

// PublishDevice difunde un evento "device" ({"event": DeviceEvent,
// "device": KnownDevice} — ver store.RecordedEvent, que ya trae esos tags).
func (h *Hub) PublishDevice(payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.publish("device", data)
}

// PublishAlert difunde el evento "alert" con la Alert completa (ver
// store.Alert, que ya trae esos tags — docs/agente-api.md, sección
// Stream: "SSE: nuevo evento event: alert con el Alert completo").
func (h *Hub) PublishAlert(payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.publish("alert", data)
}

// handleStream sirve GET /api/v1/stream.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "este servidor no soporta streaming")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.event, msg.data)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
