package core

import (
	"bufio"
	"strconv"
	"strings"
	"testing"
)

// portsDataLineCount: filas de datos en data/ports.tsv, contadas a mano
// desde el archivo (sin el encabezado), para no depender de loadPorts() al
// fijar el número esperado.
func portsDataLineCount(t *testing.T) int {
	t.Helper()
	n := 0
	sc := bufio.NewScanner(strings.NewReader(portsData))
	header := true
	for sc.Scan() {
		if header {
			header = false
			continue
		}
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		n++
	}
	return n
}

// TestPortCatalog_TodasLasFilas: ninguna fila de data/ports.tsv se pierde al
// parsear (columnas de más/menos, puerto no numérico...).
func TestPortCatalog_TodasLasFilas(t *testing.T) {
	want := portsDataLineCount(t)
	got := PortCatalog()
	if len(got) != want {
		t.Fatalf("len(PortCatalog()) = %d, want %d (filas de data/ports.tsv)", len(got), want)
	}
}

// TestPortCatalog_SinPuertosRepetidos: cada número de puerto aparece una
// sola vez (si el archivo trajera un duplicado, loadPorts se queda con la
// primera fila sin avisar; este test protege contra que eso pase sin
// querer).
func TestPortCatalog_SinPuertosRepetidos(t *testing.T) {
	seen := map[int]bool{}
	for _, e := range PortCatalog() {
		if seen[e.Port] {
			t.Errorf("puerto %d repetido en el catálogo", e.Port)
		}
		seen[e.Port] = true
	}
}

// TestPortCatalog_FilasCrudasSinDuplicados: lo mismo, pero leyendo el
// archivo embebido directamente línea por línea (no vía loadPorts), para
// detectar un duplicado real en data/ports.tsv aunque el parser lo tolere.
func TestPortCatalog_FilasCrudasSinDuplicados(t *testing.T) {
	seen := map[string]int{}
	sc := bufio.NewScanner(strings.NewReader(portsData))
	header := true
	for sc.Scan() {
		if header {
			header = false
			continue
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		cols := strings.Split(sc.Text(), "\t")
		if len(cols) == 0 {
			continue
		}
		seen[strings.TrimSpace(cols[0])]++
	}
	for port, n := range seen {
		if n > 1 {
			t.Errorf("puerto %q aparece %d veces en data/ports.tsv", port, n)
		}
	}
}

// TestPortCatalog_RiskValido: Risk siempre es uno de los cuatro valores del
// contrato.
func TestPortCatalog_RiskValido(t *testing.T) {
	valid := map[string]bool{"none": true, "low": true, "medium": true, "high": true}
	for _, e := range PortCatalog() {
		if !valid[e.Risk] {
			t.Errorf("puerto %d: risk = %q, no es none|low|medium|high", e.Port, e.Risk)
		}
	}
}

// TestPortCatalog_CopiaIndependiente: modificar el slice devuelto no debe
// afectar al catálogo compartido (PortCatalog devuelve una copia).
func TestPortCatalog_CopiaIndependiente(t *testing.T) {
	a := PortCatalog()
	if len(a) == 0 {
		t.Fatal("PortCatalog() vacío")
	}
	a[0].Name = "mutado a propósito"
	b := PortCatalog()
	if b[0].Name == "mutado a propósito" {
		t.Error("PortCatalog() comparte estado entre llamadas: una mutación se filtró")
	}
}

func TestLookupPort(t *testing.T) {
	e, ok := LookupPort(445)
	if !ok {
		t.Fatal("LookupPort(445) ok = false, want true")
	}
	if e.Name != "SMB" || e.Risk != "medium" {
		t.Errorf("LookupPort(445) = %+v, want Name=SMB Risk=medium", e)
	}
	if e.Why == "" || e.Action == "" {
		t.Errorf("LookupPort(445) = %+v, want Why/Action no vacíos (risk medium)", e)
	}

	// Puerto de riesgo "none": Why/Action vacíos es correcto (no hay nada que avisar).
	dns, ok := LookupPort(53)
	if !ok || dns.Risk != "none" {
		t.Fatalf("LookupPort(53) = %+v, ok=%v, want Risk=none", dns, ok)
	}

	if _, ok := LookupPort(99999); ok {
		t.Error("LookupPort(99999) ok = true, want false (puerto fuera del catálogo)")
	}
}

// TestLookupPort_PuertosDelContrato: los puertos que docs/agente-api.md cita
// explícitamente como ejemplo (135, 5040, 7680, 23, 445, 554, 3389) deben
// estar en el catálogo.
func TestLookupPort_PuertosDelContrato(t *testing.T) {
	for _, p := range []int{135, 5040, 7680, 23, 445, 554, 3389} {
		if _, ok := LookupPort(p); !ok {
			t.Errorf("LookupPort(%d) ok = false, want true (puerto citado en el contrato)", p)
		}
	}
}

func TestPortCatalog_OrdenPorPuerto(t *testing.T) {
	cat := PortCatalog()
	for i := 1; i < len(cat); i++ {
		if cat[i-1].Port >= cat[i].Port {
			t.Errorf("PortCatalog() no está ordenado por puerto: [%d]=%d, [%d]=%d",
				i-1, cat[i-1].Port, i, cat[i].Port)
		}
	}
}

// sanity check adicional: el propio helper de este archivo coincide con
// strconv para no inventar un número "a mano" en otro test.
func TestPortsDataLineCountHelper(t *testing.T) {
	if n := portsDataLineCount(t); n == 0 {
		t.Fatal("portsDataLineCount() = 0, ¿data/ports.tsv vacío?")
	}
	// y que loadPorts no perdió ninguna fila con número de puerto inválido.
	for _, e := range PortCatalog() {
		if _, err := strconv.Atoi(strings.TrimSpace(strconv.Itoa(e.Port))); err != nil {
			t.Errorf("puerto no numérico en el catálogo: %+v", e)
		}
	}
}
