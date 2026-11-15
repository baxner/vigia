package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/store"
)

// pairCmd imprime un código de vinculación de 6 dígitos, válido 5 minutos
// (ver docs/agente-api.md, "Vinculación").
func pairCmd(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	port := fs.Int("port", defaultPort, "puerto HTTP del agente")
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	code, expiresAt, source, err := requestPairingCode(*port, *dataDir)
	if err != nil {
		return err
	}

	fmt.Printf("Código de vinculación: %s\n", code)
	fmt.Printf("Válido hasta las %s (5 minutos).\n", expiresAt.Local().Format("15:04:05"))
	if source == "offline" {
		fmt.Println("(el agente no está corriendo ahora: el código quedó guardado en la base;")
		fmt.Println(" arranca `vigia-agent run` antes de que el teléfono lo mande, o se vencerá igual.)")
	}
	fmt.Println("En la app: buscar el agente (o su IP:puerto) y escribir este código.")
	return nil
}

// pairCodeHTTPResponse: lo que devuelve POST /internal/pair-code (no es
// parte del contrato con la app — ver agent/api/pairing.go).
type pairCodeHTTPResponse struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

// requestPairingCode decide cómo conseguir el código: primero le pregunta
// al agente ya corriendo por el endpoint local solo-loopback; si no
// responde (no está corriendo, o no en ese puerto), lo genera escribiendo
// directo en la base de datos. Las dos rutas terminan en el mismo lugar —
// la tabla pairing_codes— así que si el agente arranca después, dentro de
// la ventana de 5 minutos, valida igual el código que ya se imprimió: no
// hace falta que "lo sepa" en memoria, TryConsumePairingCode relee la fila
// pendiente en cada intento.
func requestPairingCode(port int, dataDir string) (code string, expiresAt time.Time, source string, err error) {
	if c, exp, httpErr := requestPairingCodeHTTP(port); httpErr == nil {
		return c, exp, "http", nil
	}

	paths, err := agent.DefaultPaths(dataDir)
	if err != nil {
		return "", time.Time{}, "", err
	}
	if err := paths.EnsureDirs(); err != nil {
		return "", time.Time{}, "", err
	}
	st, err := store.Open(context.Background(), paths.DBPath())
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("el agente no respondió en el puerto %d, y no se pudo abrir la base en %s: %w", port, paths.DataDir, err)
	}
	defer st.Close()

	pc, err := st.CreatePairingCode(context.Background(), time.Now().UTC())
	if err != nil {
		return "", time.Time{}, "", err
	}
	return pc.Code, pc.ExpiresAt, "offline", nil
}

// requestPairingCodeHTTP le pide el código al agente ya corriendo, por
// HTTPS (ver TLS v1.5): loopbackInsecureClient (definido en status.go) no
// valida el certificado porque este CLI corre en la misma máquina que lo
// generó — nada que fijar, a diferencia de la app.
func requestPairingCodeHTTP(port int) (code string, expiresAt time.Time, err error) {
	url := fmt.Sprintf("https://127.0.0.1:%d/internal/pair-code", port)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	resp, err := loopbackInsecureClient.Do(req)
	if err != nil {
		return "", time.Time{}, err // agente no corriendo (o no en ese puerto): esperado, no es un error para quien llama
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("/internal/pair-code: status %d", resp.StatusCode)
	}

	var body pairCodeHTTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", time.Time{}, fmt.Errorf("respuesta de /internal/pair-code: %w", err)
	}
	at, err := time.Parse(time.RFC3339Nano, body.ExpiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("expiresAt de /internal/pair-code: %w", err)
	}
	return body.Code, at, nil
}
