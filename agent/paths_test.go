package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultPaths_ConOverride(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join(dir, "sub")

	p, err := DefaultPaths(rel)
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	if p.DataDir != rel {
		t.Fatalf("DataDir = %q, want %q", p.DataDir, rel)
	}
	if p.LogDir != rel {
		t.Fatalf("LogDir = %q, want %q (con override, datos y logs comparten directorio)", p.LogDir, rel)
	}
	if p.DBPath() != filepath.Join(rel, "agent.db") {
		t.Fatalf("DBPath() = %q", p.DBPath())
	}
	if p.IdentityPath() != filepath.Join(rel, "agent.json") {
		t.Fatalf("IdentityPath() = %q", p.IdentityPath())
	}
	if p.LogPath() != filepath.Join(rel, "agent.log") {
		t.Fatalf("LogPath() = %q", p.LogPath())
	}
}

func TestDefaultPaths_SinOverrideUsaElDelSistema(t *testing.T) {
	p, err := DefaultPaths("")
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	if !filepath.IsAbs(p.DataDir) || !filepath.IsAbs(p.LogDir) {
		t.Fatalf("rutas no absolutas: %+v", p)
	}

	switch runtime.GOOS {
	case "darwin":
		if p.DataDir == p.LogDir {
			t.Fatal("en macOS, datos y logs van en directorios distintos (ver docs/agente-api.md)")
		}
		for _, want := range []string{"Library", "Application Support", "Vigia"} {
			if !strings.Contains(p.DataDir, want) {
				t.Fatalf("DataDir = %q, esperaba contener %q", p.DataDir, want)
			}
		}
	default:
		if p.DataDir != p.LogDir {
			t.Fatalf("fuera de macOS, datos y logs comparten directorio; got DataDir=%q LogDir=%q", p.DataDir, p.LogDir)
		}
	}
}

func TestEnsureDirs_CreaConPermisos0700(t *testing.T) {
	base := t.TempDir()
	p := Paths{
		DataDir: filepath.Join(base, "datos"),
		LogDir:  filepath.Join(base, "logs"),
	}

	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	for _, dir := range []string{p.DataDir, p.LogDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat(%q): %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q no es un directorio", dir)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Fatalf("permisos de %q = %o, want 700", dir, perm)
		}
	}
}
