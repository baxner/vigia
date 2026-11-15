package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWebhooks_CrearListarBorrar(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	hook, err := s.CreateWebhook(ctx, "http://homeassistant.local:8123/api/webhook/vigia", []string{"NEW_DEVICE"}, "shh", now)
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if hook.ID == 0 {
		t.Fatal("esperaba un id != 0")
	}
	if hook.Secret != "shh" {
		t.Fatalf("Secret = %q, want %q (en el valor Go, no en la API)", hook.Secret, "shh")
	}

	list, err := s.Webhooks(ctx)
	if err != nil {
		t.Fatalf("Webhooks: %v", err)
	}
	if len(list) != 1 || list[0].ID != hook.ID || list[0].URL != hook.URL {
		t.Fatalf("Webhooks = %+v", list)
	}
	if len(list[0].Types) != 1 || list[0].Types[0] != "NEW_DEVICE" {
		t.Fatalf("Types = %v, want [NEW_DEVICE]", list[0].Types)
	}
	if list[0].LastStatus != nil || list[0].LastError != nil {
		t.Fatalf("lastStatus/lastError deberían ser nil antes de cualquier entrega: %+v", list[0])
	}

	got, err := s.WebhookByID(ctx, hook.ID)
	if err != nil {
		t.Fatalf("WebhookByID: %v", err)
	}
	if got.Secret != "shh" {
		t.Fatalf("WebhookByID.Secret = %q, want shh", got.Secret)
	}

	if err := s.DeleteWebhook(ctx, hook.ID); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	if _, err := s.WebhookByID(ctx, hook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("WebhookByID tras borrar: err=%v, want ErrNotFound", err)
	}
	if err := s.DeleteWebhook(ctx, hook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteWebhook de nuevo: err=%v, want ErrNotFound", err)
	}
}

func TestWebhooks_TypesVacioAceptaCualquiera(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	hook, err := s.CreateWebhook(ctx, "http://example.local/hook", nil, "", time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if len(hook.Types) != 0 {
		t.Fatalf("Types = %v, want vacío", hook.Types)
	}
	if hook.Secret != "" {
		t.Fatalf("Secret = %q, want vacío", hook.Secret)
	}
}

func TestWebhook_Accepts(t *testing.T) {
	sinFiltro := Webhook{}
	if !sinFiltro.Accepts("NEW_DEVICE") || !sinFiltro.Accepts("AGENT_BLIND") {
		t.Fatal("Types vacío debe aceptar cualquier tipo")
	}

	conFiltro := Webhook{Types: []string{"NEW_DEVICE", "AGENT_BLIND"}}
	if !conFiltro.Accepts("NEW_DEVICE") {
		t.Fatal("NEW_DEVICE debería aceptarse")
	}
	if conFiltro.Accepts("IMPORTANT_OFFLINE") {
		t.Fatal("IMPORTANT_OFFLINE no debería aceptarse: no está en Types")
	}
}

// El secreto jamás debe aparecer en el JSON de un Webhook, ni como valor
// ni como clave (json:"-" lo excluye del todo) — esta es la frontera de
// seguridad real del contrato ("sin secret en la respuesta").
func TestWebhook_SecretNuncaEnJSON(t *testing.T) {
	w := Webhook{ID: 1, URL: "http://x", Secret: "super-secreto-no-filtrar"}
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "super-secreto-no-filtrar") {
		t.Fatalf("el JSON contiene el secreto: %s", s)
	}
	if strings.Contains(s, `"secret"`) || strings.Contains(s, `"Secret"`) {
		t.Fatalf("el JSON tiene una clave secret/Secret, debería estar excluida del todo: %s", s)
	}
}

func TestUpdateWebhookResult_StatusYError(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	hook, err := s.CreateWebhook(ctx, "http://example.local/hook", nil, "", time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	status := 200
	if err := s.UpdateWebhookResult(ctx, hook.ID, &status, nil); err != nil {
		t.Fatalf("UpdateWebhookResult: %v", err)
	}
	got, err := s.WebhookByID(ctx, hook.ID)
	if err != nil {
		t.Fatalf("WebhookByID: %v", err)
	}
	if got.LastStatus == nil || *got.LastStatus != 200 {
		t.Fatalf("LastStatus = %v, want 200", got.LastStatus)
	}
	if got.LastError != nil {
		t.Fatalf("LastError = %v, want nil", got.LastError)
	}

	msg := "connection refused"
	if err := s.UpdateWebhookResult(ctx, hook.ID, nil, &msg); err != nil {
		t.Fatalf("UpdateWebhookResult (error): %v", err)
	}
	got2, err := s.WebhookByID(ctx, hook.ID)
	if err != nil {
		t.Fatalf("WebhookByID: %v", err)
	}
	if got2.LastStatus != nil {
		t.Fatalf("LastStatus = %v, want nil tras un fallo de transporte", got2.LastStatus)
	}
	if got2.LastError == nil || *got2.LastError != msg {
		t.Fatalf("LastError = %v, want %q", got2.LastError, msg)
	}
}
