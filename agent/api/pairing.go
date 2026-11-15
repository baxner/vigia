package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/baxner/vigia/agent/store"
)

// pairRequest: cuerpo de POST /api/v1/pair (ver docs/agente-api.md).
type pairRequest struct {
	Code   string `json:"code"`
	Client string `json:"client"`
}

type pairResponse struct {
	Token     string `json:"token"`
	AgentID   string `json:"agentId"`
	AgentName string `json:"agentName"`
	// CertSHA256: huella SHA-256 (hex) del certificado TLS del agente (ver
	// TLS v1.5, docs/agente-api.md): la app la fija al vincular y la usa
	// para todas las llamadas de ahí en adelante.
	CertSHA256 string `json:"certSha256"`
}

// handlePair sirve POST /api/v1/pair (sin token: es como se consigue uno).
func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if !s.pairLimiter.allow(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "demasiados intentos de vinculación, espera un minuto")
		return
	}
	var req pairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Client = strings.TrimSpace(req.Client)
	if req.Code == "" || req.Client == "" {
		writeError(w, http.StatusBadRequest, "code y client son obligatorios")
		return
	}

	now := time.Now().UTC()
	result, err := s.store.TryConsumePairingCode(r.Context(), req.Code, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	if result != store.PairingOK {
		// Mismo mensaje para todos los motivos (vencido, agotado, equivocado,
		// sin código pendiente): no le decimos a quien ataca cuál fue.
		writeError(w, http.StatusUnauthorized, "código inválido o vencido")
		return
	}

	token, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	if _, err := s.store.CreateClient(r.Context(), req.Client, store.HashToken(token), now); err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}

	writeJSON(w, http.StatusOK, pairResponse{Token: token, AgentID: s.identity.ID, AgentName: s.identity.Name, CertSHA256: s.certSHA256})
}

// pairCodeResponse: respuesta de /internal/pair-code (no es del contrato
// con la app, ver auth.go y server.go).
type pairCodeResponse struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

// handlePairCode sirve POST /internal/pair-code: genera un código nuevo
// (invalidando cualquiera pendiente) y lo entrega a `vigia-agent pair`.
func (s *Server) handlePairCode(w http.ResponseWriter, r *http.Request) {
	pc, err := s.store.CreatePairingCode(r.Context(), time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error interno")
		return
	}
	writeJSON(w, http.StatusOK, pairCodeResponse{
		Code:      pc.Code,
		ExpiresAt: pc.ExpiresAt.UTC().Format(time.RFC3339Nano),
	})
}

// randomToken: 32 bytes de crypto/rand en hex (64 caracteres, "<64 hex>"
// del contrato).
func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
