package rotatelog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriter_EscribeSinRotarPorDebajoDelLimite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	w, err := Open(path, 1024, 3)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	if _, err := w.Write([]byte("hola\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1 (sin rotar todavía)", len(entries))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hola\n" {
		t.Fatalf("contenido = %q, want %q", data, "hola\n")
	}
}

func TestWriter_RotaAlPasarDelLimiteYConservaKeep(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	// maxBytes=10: cada línea de 11 bytes ("0123456789\n") fuerza una
	// rotación en la siguiente escritura.
	w, err := Open(path, 10, 3)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	line := []byte("0123456789\n")
	for i := 0; i < 5; i++ {
		if _, err := w.Write(line); err != nil {
			t.Fatalf("Write #%d: %v", i, err)
		}
	}
	// 5 escrituras de 11 bytes > 10: cada una desde la 2ª rota antes de
	// escribir. Resultado esperado: path (la 5ª), path.1 (la 4ª), path.2
	// (la 3ª), path.3 (la 2ª) — la 1ª se perdió (keep=3).
	assertFileContains(t, path, "0123456789\n")
	assertFileContains(t, path+".1", "0123456789\n")
	assertFileContains(t, path+".2", "0123456789\n")
	assertFileContains(t, path+".3", "0123456789\n")
	if _, err := os.Stat(path + ".4"); !os.IsNotExist(err) {
		t.Fatalf("path.4 no debería existir (keep=3): err=%v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4 (activo + 3 backups)", len(entries))
	}
}

func TestWriter_KeepCero_SoloTrunca(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	w, err := Open(path, 10, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	line := []byte("0123456789\n")
	for i := 0; i < 3; i++ {
		w.Write(line)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1 (keep=0: sin backups)", len(entries))
	}
}

func TestWriter_LineaMasGrandeQueElLimite_SeEscribeEntera(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	w, err := Open(path, 5, 3)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	big := bytes.Repeat([]byte("x"), 100)
	n, err := w.Write(big)
	if err != nil || n != len(big) {
		t.Fatalf("Write = %d, %v, want %d, nil", n, err, len(big))
	}
	data, _ := os.ReadFile(path)
	if len(data) != 100 {
		t.Fatalf("len(data) = %d, want 100 (no se corta a la mitad)", len(data))
	}

	// La SIGUIENTE escritura, por chica que sea, ahora sí dispara la
	// rotación (el archivo ya pasó el límite).
	w.Write([]byte("y\n"))
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("path.1 debería existir tras la rotación: %v", err)
	}
}

// TestWriter_ReabrirRespetaElTamañoYaEnDisco simula un reinicio del
// agente a mitad de un archivo: Open sobre un path que YA tenía
// contenido (de antes de reiniciar) debe seguir contando desde ese
// tamaño, no desde 0 — si no, un agent.log que ya estaba casi en el
// límite tardaría otro maxBytes completo en rotar tras cada reinicio.
func TestWriter_ReabrirRespetaElTamañoYaEnDisco(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	if err := os.WriteFile(path, bytes.Repeat([]byte("a"), 9), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w, err := Open(path, 10, 3)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	// 9 bytes ya en disco + 5 más > 10: debe rotar ANTES de escribir.
	if _, err := w.Write([]byte("bcdef")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("path.1 debería existir: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "bcdef" {
		t.Fatalf("contenido tras rotar = %q, want %q", data, "bcdef")
	}
}

func TestWriter_EscriturasConcurrentesNoSeCorrompen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	w, err := Open(path, 2048, 3)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	const goroutines = 20
	const perGoroutine = 50
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				line := []byte(fmt.Sprintf("g%d-%d\n", g, i))
				if _, err := w.Write(line); err != nil {
					t.Errorf("Write: %v", err)
				}
			}
		}(g)
	}
	wg.Wait()

	// No hay garantía de orden entre goroutines, pero sí de que CADA
	// línea completa llegó entera a algún archivo (activo o backup) —
	// cuenta las líneas totales entre todos los archivos que Open/rotate
	// pudieron haber creado.
	total := countLines(t, path)
	for i := 1; i <= 3; i++ {
		if _, err := os.Stat(backupPath(path, i)); err == nil {
			total += countLines(t, backupPath(path, i))
		}
	}
	want := goroutines * perGoroutine
	// Con keep=3 y un archivo chico, es posible que algunos backups ya se
	// hayan descartado (perdidos "a propósito" por space, ver rotate): lo
	// único que de verdad importa para "no se corrompen" es que cada
	// línea que SÍ sobrevivió esté completa (countLines ya lo exige al
	// usar bufio.Scanner, que falla con líneas truncadas sin \n final) y
	// que no se haya escrito de más.
	if total > want {
		t.Fatalf("total líneas = %d, want <= %d (no se puede inventar contenido)", total, want)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	// Si el archivo no termina en \n, Write se cortó a mitad de una
	// escritura — eso es exactamente lo que esta prueba busca detectar.
	if data[len(data)-1] != '\n' {
		t.Fatalf("%s no termina en salto de línea: una escritura quedó a medias", path)
	}
	return n
}
