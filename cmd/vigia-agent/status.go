package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/baxner/vigia/agent"
)

// infoHTTPResponse: lo que devuelve GET /api/v1/info (ver agent/api).
type infoHTTPResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Version         string  `json:"version"`
	ScanIntervalSec int     `json:"scanIntervalSec"`
	LastScanAt      *string `json:"lastScanAt"`
	CertSHA256      string  `json:"certSha256"`
}

// statusCmd resume el estado del agente: si está corriendo (vía
// GET /api/v1/info, sin token) y lo que hay en la base, esté corriendo o
// no.
func statusCmd(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	port := fs.Int("port", defaultPort, "puerto HTTP del agente")
	dataDir := fs.String("data", "", "directorio de datos (vacío = el del sistema)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	paths, err := agent.DefaultPaths(*dataDir)
	if err != nil {
		return err
	}
	fmt.Printf("Datos: %s\n", paths.DataDir)
	fmt.Printf("Logs:  %s\n", paths.LogDir)

	if info, err := fetchInfo(*port); err == nil {
		fmt.Printf("Corriendo: sí (puerto %d)\n", *port)
		fmt.Printf("  id=%s  name=%s  version=%s  scanIntervalSec=%d\n", info.ID, info.Name, info.Version, info.ScanIntervalSec)
		if info.LastScanAt != nil {
			fmt.Printf("  último escaneo: %s\n", *info.LastScanAt)
		} else {
			fmt.Println("  último escaneo: todavía ninguno")
		}
		if info.CertSHA256 != "" {
			fmt.Printf("  certificado TLS: %s (comprobar con `vigia-agent cert`)\n", info.CertSHA256)
		}
	} else {
		fmt.Printf("Corriendo: no (%v)\n", err)
	}

	st, closeFn, err := openStoreForCLI(*dataDir)
	if err != nil {
		fmt.Printf("Base de datos: no se pudo abrir (%v)\n", err)
		return nil
	}
	defer closeFn()
	stats, err := st.Stats(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("Redes: %d   Equipos: %d (en línea: %d)\n", stats.Networks, stats.Devices, stats.OnlineDevices)
	return nil
}

// loopbackInsecureClient: para hablarle al propio agente en 127.0.0.1 por
// HTTPS (ver TLS v1.5) sin tener que cargar ni comparar su huella — es el
// mismo proceso que lo generó, en la misma máquina, nada que fijar aquí
// (a diferencia de la app, que sí valida la huella porque le habla por la
// red). InsecureSkipVerify es a propósito y solo para este cliente de CLI.
var loopbackInsecureClient = &http.Client{
	Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
}

func fetchInfo(port int) (infoHTTPResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/api/v1/info", port), nil)
	if err != nil {
		return infoHTTPResponse{}, err
	}
	resp, err := loopbackInsecureClient.Do(req)
	if err != nil {
		return infoHTTPResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return infoHTTPResponse{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out infoHTTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return infoHTTPResponse{}, err
	}
	return out, nil
}
