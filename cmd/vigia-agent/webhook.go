package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/webhooks"
)

// webhookCmd despacha los subcomandos de `vigia-agent webhook` (ver
// docs/agente-api.md, sección Webhooks). Como el resto de la CLI, los
// flags de cada subcomando van ANTES de los argumentos posicionales:
// flag.FlagSet deja de interpretar flags en cuanto encuentra el primer
// argumento que no empieza con "-" (ver revokeCmd/tokensCmd en
// tokens.go). Así que es "webhook add -types NEW_DEVICE <url>", no
// "webhook add <url> -types NEW_DEVICE" (con los flags después, -types
// quedaría como un argumento posicional de más y fs.NArg() fallaría).
func webhookCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: vigia-agent webhook add|list|rm|test ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return webhookAddCmd(rest)
	case "list":
		return webhookListCmd(rest)
	case "rm":
		return webhookRmCmd(rest)
	case "test":
		return webhookTestCmd(rest)
	default:
		return fmt.Errorf("subcomando desconocido: %q (add|list|rm|test)", sub)
	}
}

func webhookAddCmd(args []string) error {
	fs := flag.NewFlagSet("webhook add", flag.ExitOnError)
	types := fs.String("types", "", "tipos separados por coma (vacío = todos: NEW_DEVICE,IMPORTANT_OFFLINE,IMPORTANT_ONLINE,AGENT_BLIND,AGENT_RECOVERED)")
	secret := fs.String("secret", "", "secreto para firmar cada entrega (X-Vigia-Signature)")
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("uso: vigia-agent webhook add [-types T1,T2] [-secret S] [-data DIR] <url>")
	}
	url := strings.TrimSpace(fs.Arg(0))
	if err := webhooks.ValidateWebhookURL(url); err != nil {
		return fmt.Errorf("url inválida: %w", err)
	}
	var typeList []string
	if *types != "" {
		typeList = strings.Split(*types, ",")
	}

	st, closeFn, err := openStoreForCLI(*dataDir)
	if err != nil {
		return err
	}
	defer closeFn()

	hook, err := st.CreateWebhook(context.Background(), url, typeList, *secret, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Printf("Webhook #%d creado: %s\n", hook.ID, hook.URL)
	return nil
}

func webhookListCmd(args []string) error {
	fs := flag.NewFlagSet("webhook list", flag.ExitOnError)
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, closeFn, err := openStoreForCLI(*dataDir)
	if err != nil {
		return err
	}
	defer closeFn()

	hooks, err := st.Webhooks(context.Background())
	if err != nil {
		return err
	}
	if len(hooks) == 0 {
		fmt.Println("(sin webhooks registrados todavía)")
		return nil
	}
	for _, h := range hooks {
		types := "todos"
		if len(h.Types) > 0 {
			types = strings.Join(h.Types, ",")
		}
		secreto := "sin secreto"
		if h.Secret != "" {
			secreto = "con secreto"
		}
		estado := "sin entregas todavía"
		switch {
		case h.LastStatus != nil:
			estado = fmt.Sprintf("último status=%d", *h.LastStatus)
		case h.LastError != nil:
			estado = "último error=" + *h.LastError
		}
		fmt.Printf("#%d  %s  tipos=%s  %s  %s\n", h.ID, h.URL, types, secreto, estado)
	}
	return nil
}

func webhookRmCmd(args []string) error {
	fs := flag.NewFlagSet("webhook rm", flag.ExitOnError)
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("uso: vigia-agent webhook rm [-data DIR] <id>")
	}
	id, err := parseWebhookID(fs.Arg(0))
	if err != nil {
		return err
	}

	st, closeFn, err := openStoreForCLI(*dataDir)
	if err != nil {
		return err
	}
	defer closeFn()

	if err := st.DeleteWebhook(context.Background(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no hay ningún webhook #%d", id)
		}
		return err
	}
	fmt.Printf("Webhook #%d borrado.\n", id)
	return nil
}

// webhookTestCmd manda UN intento inmediato (sin reintentos, igual que
// POST /api/v1/webhooks/{id}/test — mismo motivo: es un botón de
// "probar ahora") con la alerta sintética de agent/alerts.TestAlert.
func webhookTestCmd(args []string) error {
	fs := flag.NewFlagSet("webhook test", flag.ExitOnError)
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	name := fs.String("name", "", "nombre del agente a reportar en el payload (vacío = el de agent.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("uso: vigia-agent webhook test [-data DIR] <id>")
	}
	id, err := parseWebhookID(fs.Arg(0))
	if err != nil {
		return err
	}

	paths, err := agent.DefaultPaths(*dataDir)
	if err != nil {
		return err
	}
	identity, err := agent.LoadOrCreateIdentity(paths.DataDir, *name)
	if err != nil {
		return fmt.Errorf("identidad: %w", err)
	}

	st, closeFn, err := openStoreForCLI(*dataDir)
	if err != nil {
		return err
	}
	defer closeFn()

	hook, err := st.WebhookByID(context.Background(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no hay ningún webhook #%d", id)
		}
		return err
	}

	test := alerts.TestAlert(time.Now().UTC())
	body, err := json.Marshal(webhooks.Payload{
		Event: "alert",
		Alert: store.Alert{Type: test.Type, Severity: test.Severity, At: test.At, Title: test.Title, Body: test.Body},
		Agent: webhooks.AgentInfo{ID: identity.ID, Name: identity.Name},
	})
	if err != nil {
		return err
	}
	deliveryID, err := webhooks.NewDeliveryID()
	if err != nil {
		return err
	}

	ctx := context.Background()
	sender := webhooks.NewSender(version)
	target := webhooks.Target{ID: hook.ID, URL: hook.URL, Secret: hook.Secret}
	status, sendErr := sender.Attempt(ctx, target, body, string(test.Type), deliveryID)
	if sendErr != nil {
		msg := sendErr.Error()
		_ = st.UpdateWebhookResult(ctx, hook.ID, nil, &msg)
		return fmt.Errorf("no se pudo entregar: %w", sendErr)
	}
	_ = st.UpdateWebhookResult(ctx, hook.ID, &status, nil)
	fmt.Printf("Entregado: status=%d\n", status)
	return nil
}

func parseWebhookID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("id inválido: %q", raw)
	}
	return id, nil
}
