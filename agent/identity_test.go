package agent

import (
	"regexp"
	"testing"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDv4_FormatoYUnicidad(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := newUUIDv4()
		if err != nil {
			t.Fatalf("newUUIDv4: %v", err)
		}
		if !uuidRE.MatchString(id) {
			t.Fatalf("uuid = %q, no tiene forma de UUID v4", id)
		}
		if seen[id] {
			t.Fatalf("uuid repetido: %q", id)
		}
		seen[id] = true
	}
}

func TestLoadOrCreateIdentity_CreaNuevaConNombrePorDefecto(t *testing.T) {
	dir := t.TempDir()

	id, err := LoadOrCreateIdentity(dir, "")
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity: %v", err)
	}
	if !uuidRE.MatchString(id.ID) {
		t.Fatalf("id = %q, no tiene forma de UUID v4", id.ID)
	}
	if id.Name == "" {
		t.Fatal("esperaba un nombre por defecto no vacío")
	}
	if id.CreatedAt.IsZero() {
		t.Fatal("esperaba CreatedAt distinto de cero")
	}
}

func TestLoadOrCreateIdentity_CreaNuevaConNombreExplicito(t *testing.T) {
	dir := t.TempDir()

	id, err := LoadOrCreateIdentity(dir, "Nodo-Estudio")
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity: %v", err)
	}
	if id.Name != "Nodo-Estudio" {
		t.Fatalf("name = %q, want Nodo-Estudio", id.Name)
	}
}

func TestLoadOrCreateIdentity_RecargaLaMismaIdentidad(t *testing.T) {
	dir := t.TempDir()

	primera, err := LoadOrCreateIdentity(dir, "Nodo-Estudio")
	if err != nil {
		t.Fatalf("primera LoadOrCreateIdentity: %v", err)
	}

	segunda, err := LoadOrCreateIdentity(dir, "")
	if err != nil {
		t.Fatalf("segunda LoadOrCreateIdentity: %v", err)
	}
	if segunda.ID != primera.ID {
		t.Fatalf("id = %q, want %q (debe conservarse entre reinicios)", segunda.ID, primera.ID)
	}
	if segunda.Name != "Nodo-Estudio" {
		t.Fatalf("name = %q, want Nodo-Estudio (sin -name, no debe cambiar)", segunda.Name)
	}
}

func TestLoadOrCreateIdentity_ActualizaNombreSiLlegaUnoDistinto(t *testing.T) {
	dir := t.TempDir()

	primera, err := LoadOrCreateIdentity(dir, "Nodo-Estudio")
	if err != nil {
		t.Fatalf("primera LoadOrCreateIdentity: %v", err)
	}

	renombrada, err := LoadOrCreateIdentity(dir, "Nodo-Nuevo")
	if err != nil {
		t.Fatalf("segunda LoadOrCreateIdentity: %v", err)
	}
	if renombrada.ID != primera.ID {
		t.Fatalf("id = %q, want %q (renombrar no debe cambiar el id)", renombrada.ID, primera.ID)
	}
	if renombrada.Name != "Nodo-Nuevo" {
		t.Fatalf("name = %q, want Nodo-Nuevo", renombrada.Name)
	}

	// Y quedó persistido.
	releida, err := LoadOrCreateIdentity(dir, "")
	if err != nil {
		t.Fatalf("tercera LoadOrCreateIdentity: %v", err)
	}
	if releida.Name != "Nodo-Nuevo" {
		t.Fatalf("name tras recargar = %q, want Nodo-Nuevo", releida.Name)
	}
}
