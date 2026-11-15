// vigia-scan: CLI de prueba del motor core. Escanea la red y lista lo encontrado.
//
//	go run ./cmd/vigia-scan                 # red por defecto, tabla
//	go run ./cmd/vigia-scan -cidr 192.168.0.0/24 -json
//	go run ./cmd/vigia-scan -noports        # sin escaneo de puertos (más rápido)
//	go run ./cmd/vigia-scan -noarp          # solo pruebas: simula Android, sin ARP/NDP
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/baxner/vigia/core"
)

func main() {
	cidr := flag.String("cidr", "", "red a escanear (vacío = autodetectar)")
	asJSON := flag.Bool("json", false, "salida JSON")
	noPorts := flag.Bool("noports", false, "no escanear puertos")
	quiet := flag.Bool("quiet", false, "sin barra de progreso")
	noARP := flag.Bool("noarp", false, "solo pruebas: simula Android, sin tabla ARP/NDP")
	flag.Parse()

	opts := core.ScanOptions{CIDR: *cidr, SkipNeighborTable: *noARP}
	if *noPorts {
		opts.Ports = []int{}
	}
	if !*quiet {
		last := ""
		opts.OnProgress = func(stage string, done, total int) {
			if stage == "sweep" && (done%16 == 0 || done == total) {
				msg := fmt.Sprintf("\r%s %d/%d", stage, done, total)
				if msg != last {
					fmt.Fprint(os.Stderr, msg)
					last = msg
				}
			}
		}
	}

	start := time.Now()
	res, err := core.Scan(context.Background(), opts)
	fmt.Fprint(os.Stderr, "\r\033[K")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(res)
		return
	}

	fmt.Printf("Red %s  ·  yo %s  ·  gateway %s  ·  %d dispositivos  ·  %s\n\n",
		res.Subnet, res.Self, res.Gateway, len(res.Devices), time.Since(start).Round(time.Millisecond))
	fmt.Printf("%-15s %-17s %-20s %-22s %-10s %s\n", "IP", "MAC", "FABRICANTE", "NOMBRE", "TIPO", "PUERTOS")
	fmt.Println(strings.Repeat("-", 110))
	for _, d := range res.Devices {
		ports := ""
		for i, p := range d.OpenPorts {
			if i > 0 {
				ports += ","
			}
			ports += fmt.Sprint(p)
		}
		tipo := d.Identity.Type
		if d.Identity.Confidence > 0 {
			tipo = fmt.Sprintf("%s(%d)", tipo, d.Identity.Confidence)
		}
		fmt.Printf("%-15s %-17s %-20.20s %-22.22s %-10s %s\n",
			d.IP, d.MAC, d.Vendor, firstNonEmpty(d.Hostname, d.Identity.Model), tipo, ports)
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
