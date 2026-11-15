package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/webhooks"
)

func TestWebhooks_CrearListarBorrar(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	resp := env.doJSON(http.MethodPost, "/api/v1/webhooks", token,
		webhookCreateRequest{URL: "http://homeassistant.local:8123/api/webhook/vigia", Types: []string{"NEW_DEVICE"}, Secret: "no-filtrar-esto"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	rawBody := readAllString(t, resp.Body)
	if strings.Contains(rawBody, "no-filtrar-esto") || strings.Contains(rawBody, "secret") {
		t.Fatalf("la respuesta de creación filtró el secreto: %s", rawBody)
	}
	var created store.Webhook
	if err := json.Unmarshal([]byte(rawBody), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == 0 || created.URL == "" {
		t.Fatalf("created = %+v", created)
	}

	resp2 := env.doJSON(http.MethodGet, "/api/v1/webhooks", token, nil)
	defer resp2.Body.Close()
	rawList := readAllString(t, resp2.Body)
	if strings.Contains(rawList, "no-filtrar-esto") {
		t.Fatalf("GET /webhooks filtró el secreto: %s", rawList)
	}
	var list []store.Webhook
	json.Unmarshal([]byte(rawList), &list)
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}

	resp3 := env.doJSON(http.MethodDelete, "/api/v1/webhooks/"+strconv.FormatInt(created.ID, 10), token, nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp3.StatusCode)
	}

	resp4 := env.doJSON(http.MethodGet, "/api/v1/webhooks", token, nil)
	defer resp4.Body.Close()
	var list2 []store.Webhook
	json.NewDecoder(resp4.Body).Decode(&list2)
	if len(list2) != 0 {
		t.Fatalf("tras borrar: list = %+v, want vacío", list2)
	}

	resp5 := env.doJSON(http.MethodDelete, "/api/v1/webhooks/"+strconv.FormatInt(created.ID, 10), token, nil)
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusNotFound {
		t.Fatalf("borrar de nuevo: status = %d, want 404", resp5.StatusCode)
	}
}

func readAllString(t *testing.T, r io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(data)
}

func TestWebhooksCreate_URLInvalida(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	for _, url := range []string{
		"ftp://x",
		"not-a-url",
		"",
		"http:///sin-host",
		"https://user:secret@example.com/hook",
	} {
		resp := env.doJSON(http.MethodPost, "/api/v1/webhooks", token, webhookCreateRequest{URL: url})
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("url=%q: status = %d, want 400", url, resp.StatusCode)
		}
	}
}

func TestWebhookDelete_IDInexistente(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	resp := env.doJSON(http.MethodDelete, "/api/v1/webhooks/999999", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestWebhookTest_IDInexistente(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	resp := env.doJSON(http.MethodPost, "/api/v1/webhooks/999999/test", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestWebhookTest_EntregaFirmadaAUnServidorDePrueba registra un webhook
// apuntando a un httptest.Server propio (no el del agente) y llama a
// POST /api/v1/webhooks/{id}/test: verifica que llega un POST firmado de
// verdad y que el resultado queda guardado (lastStatus).
func TestWebhookTest_EntregaFirmadaAUnServidorDePrueba(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewServer(Deps{
		Store:    st,
		Identity: agent.Identity{ID: "agent-123", Name: "Test-Node"},
		Version:  "test",
		Sender: &webhooks.Sender{
			Version: "test",
			Client: &http.Client{
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			},
		},
	})
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	env := &testEnv{t: t, server: httpSrv, store: st, hub: srv.Hub()}
	token := env.pair("Tester")

	var gotSignature, gotEvent string
	var gotBody []byte
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get("X-Vigia-Signature")
		gotEvent = r.Header.Get("X-Vigia-Event")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	secret := "firma-de-prueba"
	resp := env.doJSON(http.MethodPost, "/api/v1/webhooks", token, webhookCreateRequest{URL: target.URL, Secret: secret})
	defer resp.Body.Close()
	var created store.Webhook
	json.NewDecoder(resp.Body).Decode(&created)

	resp2 := env.doJSON(http.MethodPost, "/api/v1/webhooks/"+strconv.FormatInt(created.ID, 10)+"/test", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp2.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp2.Body).Decode(&result)
	if status, ok := result["status"]; !ok || int(status.(float64)) != 200 {
		t.Fatalf("result = %+v, want status=200", result)
	}

	if gotEvent != "TEST" {
		t.Fatalf("X-Vigia-Event = %q, want TEST", gotEvent)
	}
	wantSig := "sha256=" + webhooks.Sign(secret, gotBody)
	if gotSignature != wantSig {
		t.Fatalf("X-Vigia-Signature = %q, want %q", gotSignature, wantSig)
	}

	resp3 := env.doJSON(http.MethodGet, "/api/v1/webhooks", token, nil)
	defer resp3.Body.Close()
	var list []store.Webhook
	json.NewDecoder(resp3.Body).Decode(&list)
	if len(list) != 1 || list[0].LastStatus == nil || *list[0].LastStatus != 200 {
		t.Fatalf("list = %+v, want lastStatus=200 guardado", list)
	}
}

func TestWebhookTest_BloqueaLoopbackYSanitizaError(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("el servidor loopback no debería ser alcanzado por defecto")
	}))
	defer target.Close()

	resp := env.doJSON(http.MethodPost, "/api/v1/webhooks", token, webhookCreateRequest{URL: target.URL})
	defer resp.Body.Close()
	var created store.Webhook
	json.NewDecoder(resp.Body).Decode(&created)

	resp2 := env.doJSON(http.MethodPost, "/api/v1/webhooks/"+strconv.FormatInt(created.ID, 10)+"/test", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp2.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp2.Body).Decode(&result)
	errMsg := result["error"]
	if errMsg != "destino de webhook no permitido" {
		t.Fatalf("error = %q, want %q", errMsg, "destino de webhook no permitido")
	}
	if strings.Contains(errMsg, "127.0.0.1") || strings.Contains(errMsg, "dial tcp") {
		t.Fatalf("el error filtró detalles internos del dialer: %q", errMsg)
	}
}
