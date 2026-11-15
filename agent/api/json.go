// Package api es el servidor HTTP del agente: implementa el contrato de
// docs/agente-api.md tal cual —mismos nombres de campo, mismas fechas RFC
// 3339 UTC, mismos códigos de error— apoyándose en agent/store para todo lo
// que no sea el transporte HTTP en sí.
package api

import (
	"encoding/json"
	"net/http"
)

// errorResponse: forma de error del contrato ({"error": "…"}).
type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// decodeNullableString distingue, dentro de un campo que SÍ llegó en el
// cuerpo, entre "null" (borrar: devuelve nil) y una cadena (fijar ese
// valor). Si el campo no llegó para nada, quien llama ni se mete a esta
// función — ver "PATCH /api/v1/devices/{id}" en docs/agente-api.md.
func decodeNullableString(raw json.RawMessage) (*string, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// maxBodyBytes: tope para cuerpos JSON entrantes (pair, PATCH); los reales
// pesan unos cientos de bytes.
const maxBodyBytes = 64 << 10
