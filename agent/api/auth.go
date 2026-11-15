package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/baxner/vigia/agent/store"
)

// clientAuthenticator es lo que requireToken necesita del store: buscar el
// cliente vigente por el hash del token (en tiempo constante, ver
// store.FindActiveClientByToken) y refrescar su "último uso". *store.Store
// lo cumple tal cual.
type clientAuthenticator interface {
	FindActiveClientByToken(ctx context.Context, token string) (*store.Client, error)
	TouchClient(ctx context.Context, id int64, at time.Time) error
}

// requireToken exige "Authorization: Bearer <token>" y lo resuelve contra
// auth (ver docs/agente-api.md: todos los endpoints salvo info y pair). La
// comparación en sí —en tiempo constante sobre el SHA-256— vive en
// store.FindActiveClientByToken; este middleware solo decide 401 sí/no.
func requireToken(auth clientAuthenticator, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "falta el token")
			return
		}
		client, err := auth.FindActiveClientByToken(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error interno")
			return
		}
		if client == nil {
			writeError(w, http.StatusUnauthorized, "token inválido")
			return
		}
		_ = auth.TouchClient(r.Context(), client.ID, time.Now().UTC())
		next(w, r)
	}
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

// requireLoopback restringe un endpoint a quien pregunta desde la propia
// máquina (127.0.0.1/::1). Lo usa únicamente /internal/pair-code: no es
// parte del contrato con la app (que siempre llega con token, o sin
// ninguno a /api/v1/pair), es el canal que usa `vigia-agent pair` para
// pedirle un código al agente ya corriendo sin necesitar un token (todavía
// no hay ninguno vinculado la primera vez).
func requireLoopback(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r.RemoteAddr) {
			writeError(w, http.StatusForbidden, "solo disponible desde la propia máquina")
			return
		}
		next(w, r)
	}
}

func isLoopback(remoteAddr string) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
