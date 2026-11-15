package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Paths son las rutas de datos y logs del agente (ver docs/agente-api.md,
// "Instalación en macOS", y la entrega para Linux).
type Paths struct {
	// DataDir contiene agent.db y agent.json.
	DataDir string
	// LogDir contiene agent.log. En macOS está aparte de DataDir (~/Library/Logs/Vigia);
	// en Linux y con -data es el mismo que DataDir.
	LogDir string
}

func (p Paths) DBPath() string       { return filepath.Join(p.DataDir, "agent.db") }
func (p Paths) IdentityPath() string { return filepath.Join(p.DataDir, "agent.json") }
func (p Paths) LogPath() string      { return filepath.Join(p.LogDir, "agent.log") }

// DefaultPaths son las rutas del contrato para el sistema operativo
// actual, o las que indica override si no es "" (vigia-agent run -data
// DIR: desarrollo y la prueba real sin tocar el sistema — ver la entrega).
// Con override, datos y logs comparten el mismo directorio.
func DefaultPaths(override string) (Paths, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return Paths{}, fmt.Errorf("-data %q: %w", override, err)
		}
		return Paths{DataDir: abs, LogDir: abs}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("directorio de inicio: %w", err)
	}

	if runtime.GOOS == "darwin" {
		return Paths{
			DataDir: filepath.Join(home, "Library", "Application Support", "Vigia"),
			LogDir:  filepath.Join(home, "Library", "Logs", "Vigia"),
		}, nil
	}
	// Linux (y el resto): un solo directorio para datos y logs.
	dir := filepath.Join(home, ".local", "share", "vigia")
	return Paths{DataDir: dir, LogDir: dir}, nil
}

// EnsureDirs crea DataDir y LogDir (si son distintos, los dos) con
// permisos 0700, tal como exige el contrato.
func (p Paths) EnsureDirs() error {
	dirs := []string{p.DataDir}
	if p.LogDir != p.DataDir {
		dirs = append(dirs, p.LogDir)
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("crear %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("permisos de %s: %w", dir, err)
		}
	}
	return nil
}
