// Package agent resuelve quién es el agente (id estable, nombre) y dónde
// vive (datos, logs) según el contrato (docs/agente-api.md), para que
// cmd/vigia-agent y el paquete api no tengan que conocer esos detalles de
// cada sistema operativo.
package agent

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Identity es lo mínimo que el agente necesita recordar de sí mismo entre
// reinicios: su id estable (TXT mDNS "id=", GET /api/v1/info) y su nombre
// visible (TXT "name=", normalmente el hostname del host). Vive en agent.json,
// aparte de agent.db: hace falta poder leerlo (p. ej. para anunciarse por
// mDNS) sin depender de que la base ya esté abierta.
type Identity struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// LoadOrCreateIdentity lee agent.json en dir, o lo crea si falta: un UUID
// v4 nuevo y, para el nombre, name si no es "" o si no un nombre por
// defecto a partir del hostname. Si ya existía y name no es "" y difiere
// del guardado, lo actualiza (permite `vigia-agent run -name "..."` para
// rebautizar el agente). dir debe existir ya con permisos 0700 (ver
// Paths.EnsureDirs).
func LoadOrCreateIdentity(dir, name string) (Identity, error) {
	path := filepath.Join(dir, "agent.json")

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		var id Identity
		if err := json.Unmarshal(data, &id); err != nil {
			return Identity{}, fmt.Errorf("agent.json inválido: %w", err)
		}
		if name != "" && name != id.Name {
			id.Name = name
			if err := saveIdentity(path, id); err != nil {
				return Identity{}, err
			}
		}
		return id, nil
	case os.IsNotExist(err):
		uuid, err := newUUIDv4()
		if err != nil {
			return Identity{}, err
		}
		if name == "" {
			name = defaultName()
		}
		id := Identity{ID: uuid, Name: name, CreatedAt: time.Now().UTC()}
		if err := saveIdentity(path, id); err != nil {
			return Identity{}, err
		}
		return id, nil
	default:
		return Identity{}, fmt.Errorf("leer %s: %w", path, err)
	}
}

func saveIdentity(path string, id Identity) error {
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("escribir %s: %w", path, err)
	}
	return os.Chmod(path, 0o600)
}

// defaultName: el nombre del equipo (hostname), sin el ".local" que macOS
// le agrega — es lo que la app ve como "name=" en el TXT mDNS.
func defaultName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "vigia-agent"
	}
	return strings.TrimSuffix(h, ".local")
}

// newUUIDv4 genera un UUID v4 (RFC 4122) con crypto/rand. No se agrega
// github.com/google/uuid como dependencia propia solo para esto: ya entra
// transitivamente vía modernc.org/sqlite, pero el agente no lo usa
// directamente en ningún otro lado, y 16 líneas de bit-twiddling son más
// simples de auditar que justificar por qué go.mod la vuelve directa.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // versión 4
	b[8] = (b[8] & 0x3f) | 0x80 // variante RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
