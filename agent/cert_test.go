package agent

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOrCreateCert_GeneraYPersisteEntreReinicios(t *testing.T) {
	dir := t.TempDir()

	first, err := LoadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCert (primera vez): %v", err)
	}
	if len(first.FingerprintSHA256) != 64 {
		t.Fatalf("huella = %q, quiero 64 hex", first.FingerprintSHA256)
	}
	assertCertFiles(t, dir)

	// "Reinicio": se vuelve a llamar sobre el mismo dir — debe leer lo que
	// ya había, no generar uno nuevo (ver el encargo: "se reutiliza al
	// reiniciar").
	second, err := LoadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCert (segunda vez): %v", err)
	}
	if second.FingerprintSHA256 != first.FingerprintSHA256 {
		t.Fatalf("la huella cambió entre llamadas: %q != %q", first.FingerprintSHA256, second.FingerprintSHA256)
	}
}

func TestLoadOrCreateCert_ECDSAP256YDiezAnios(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCert: %v", err)
	}
	leaf := c.TLS.Leaf
	if leaf == nil {
		parsed, err := x509.ParseCertificate(c.TLS.Certificate[0])
		if err != nil {
			t.Fatalf("parsear certificado: %v", err)
		}
		leaf = parsed
	}
	if leaf.PublicKeyAlgorithm != x509.ECDSA {
		t.Fatalf("algoritmo = %v, quiero ECDSA", leaf.PublicKeyAlgorithm)
	}
	wantMin := time.Now().AddDate(9, 11, 0) // casi 10 años, con margen por NotBefore corrido 5 min hacia atrás
	if leaf.NotAfter.Before(wantMin) {
		t.Fatalf("NotAfter = %v, quiero por lo menos %v (≈10 años)", leaf.NotAfter, wantMin)
	}
	if leaf.NotBefore.After(time.Now()) {
		t.Fatalf("NotBefore = %v, no debería ser en el futuro", leaf.NotBefore)
	}
}

// Si falta uno de los dos archivos (cert o llave, p. ej. una copia a medias),
// se regenera el par completo en vez de fallar — ver el comentario de
// LoadOrCreateCert.
func TestLoadOrCreateCert_ArchivoAMedias_Regenera(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCert: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "agent-key.pem")); err != nil {
		t.Fatalf("borrar la llave: %v", err)
	}

	second, err := LoadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCert (con la llave borrada): %v", err)
	}
	if second.FingerprintSHA256 == first.FingerprintSHA256 {
		t.Fatal("debería haber generado un certificado nuevo (la llave vieja ya no existe)")
	}
	assertCertFiles(t, dir)
}

func TestRotateCert_CambiaLaHuella(t *testing.T) {
	dir := t.TempDir()
	before, err := LoadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCert: %v", err)
	}

	after, err := RotateCert(dir)
	if err != nil {
		t.Fatalf("RotateCert: %v", err)
	}
	if after.FingerprintSHA256 == before.FingerprintSHA256 {
		t.Fatal("RotateCert debería cambiar la huella")
	}

	// Y lo que queda guardado en disco es el nuevo, no el viejo.
	reread, err := LoadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCert tras rotar: %v", err)
	}
	if reread.FingerprintSHA256 != after.FingerprintSHA256 {
		t.Fatalf("lo guardado en disco (%q) no coincide con lo que devolvió RotateCert (%q)", reread.FingerprintSHA256, after.FingerprintSHA256)
	}
}

func assertCertFiles(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"agent-cert.pem", "agent-key.pem"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("%s: permisos = %v, quiero 0600", name, perm)
		}
	}
}
