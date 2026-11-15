package rotatelog

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenUses0600Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permisos POSIX no aplican en Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	w, err := Open(path, 64, 2)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permisos iniciales = %#o, want 0o600", got)
	}
}

func TestRotateFailurePreservesWriter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("semántica de rename con archivo abierto difiere en Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	w, err := Open(path, 10, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	if _, err := w.Write([]byte("123456")); err != nil {
		t.Fatalf("primer Write: %v", err)
	}

	// Ponemos un directorio no vacío en path.1 para forzar fallo al rotar path -> path.1.
	blocker := backupPath(path, 1)
	if err := os.MkdirAll(filepath.Join(blocker, "nested"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if _, err := w.Write([]byte("abcdef")); err == nil {
		t.Fatal("se esperaba error al rotar sobre un directorio no vacío")
	}

	// Al quitar el obstáculo, w.f debe seguir vivo y poder rotar y cerrar sin error.
	if err := os.RemoveAll(blocker); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := w.Write([]byte("abcdef")); err != nil {
		t.Fatalf("Write tras recuperar rename fallido: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close tras fallo previo de rotate: %v", err)
	}
}
