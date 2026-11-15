package core

import (
	"bufio"
	"context"
	_ "embed"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// scanPorts prueba los puertos TCP `ports` de `ip` por connect(), en paralelo
// (acotado) y respetando ctx. Devuelve los ABIERTOS ordenados. Aquí, al revés que
// en el barrido de liveness, "connection refused" = CERRADO (solo cuenta el que
// acepta la conexión).
func scanPorts(ctx context.Context, ip string, ports []int, timeout time.Duration) []int {
	return scanPortsWithIface(ctx, ip, ports, timeout, "")
}

func scanPortsWithIface(ctx context.Context, ip string, ports []int, timeout time.Duration, iface string) []int {
	if len(ports) == 0 {
		return nil
	}
	timeout = orDur(timeout, DefaultPortTimeout)
	var (
		mu   sync.Mutex
		open []int
		sem  = make(chan struct{}, 64)
		wg   sync.WaitGroup
	)
	for _, p := range ports {
		select {
		case <-ctx.Done():
			wg.Wait()
			sort.Ints(open)
			return open
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			defer func() { <-sem }()
			d := newDialer(iface, timeout)
			conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, itoa(port)))
			if err == nil {
				conn.Close()
				mu.Lock()
				open = append(open, port)
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	sort.Ints(open)
	return open
}

// ---------------------------------------------------------------------------
// Catálogo de puertos (Fase 5, ver docs/plan.md): nombre, riesgo,
// por qué y qué hacer, en español de México y listo para mostrarse.
// ---------------------------------------------------------------------------

// data/ports.tsv: columnas port, name, risk (none|low|medium|high), why,
// action — "why"/"action" vacíos cuando risk es "none" (no hay nada que
// explicar). Embebido para que funcione igual en el agente, la app
// (gomobile) y los tests, sin depender de un archivo en disco — mismo
// patrón que data/apple-models.txt (ver applemodels.go).
//
//go:embed data/ports.tsv
var portsData string

// PortEntry es una fila del catálogo.
type PortEntry struct {
	Port   int    `json:"port"`
	Name   string `json:"name"`
	Risk   string `json:"risk"`   // none|low|medium|high
	Why    string `json:"why"`    // "" cuando Risk es "none": no hace falta explicar nada
	Action string `json:"action"` // "" cuando Risk es "none"
}

var (
	portsOnce  sync.Once
	portsList  []PortEntry // en el orden del archivo (ya viene ordenado por puerto)
	portsByNum map[int]PortEntry
)

// loadPorts parsea data/ports.tsv una sola vez (sync.Once, igual que
// loadAppleModels): es TSV de verdad (un \t entre columnas), no CSV, porque
// "why"/"action" pueden traer comas. La primera línea es el encabezado
// ("port\tname\trisk\twhy\taction") y se descarta.
func loadPorts() {
	portsOnce.Do(func() {
		portsByNum = make(map[int]PortEntry, 96)
		sc := bufio.NewScanner(strings.NewReader(portsData))
		header := true
		for sc.Scan() {
			line := sc.Text()
			if header {
				header = false
				continue
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			cols := strings.Split(line, "\t")
			if len(cols) != 5 {
				continue // fila mal formada: se ignora en vez de hacer panic
			}
			port, err := strconv.Atoi(strings.TrimSpace(cols[0]))
			if err != nil {
				continue
			}
			if _, dup := portsByNum[port]; dup {
				continue // puerto repetido en el archivo: se queda la primera fila
			}
			entry := PortEntry{
				Port:   port,
				Name:   strings.TrimSpace(cols[1]),
				Risk:   strings.TrimSpace(cols[2]),
				Why:    strings.TrimSpace(cols[3]),
				Action: strings.TrimSpace(cols[4]),
			}
			portsByNum[port] = entry
			portsList = append(portsList, entry)
		}
	})
}

// LookupPort busca un puerto TCP en el catálogo. ok=false si no está (no es
// necesariamente seguro: solo no lo documentamos todavía).
func LookupPort(port int) (PortEntry, bool) {
	loadPorts()
	e, ok := portsByNum[port]
	return e, ok
}

// PortCatalog devuelve el catálogo completo, en el orden de data/ports.tsv
// (por número de puerto). Copia nueva en cada llamada: quien la use puede
// modificarla sin afectar al catálogo compartido.
func PortCatalog() []PortEntry {
	loadPorts()
	out := make([]PortEntry, len(portsList))
	copy(out, portsList)
	return out
}
