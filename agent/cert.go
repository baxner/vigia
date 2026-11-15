package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// CertPath y KeyPath: dónde vive el certificado TLS propio del agente (ver
// TLS v1.5, docs/agente-api.md) — junto a agent.json y agent.db, mismos
// permisos 0600.
func (p Paths) CertPath() string { return filepath.Join(p.DataDir, "agent-cert.pem") }
func (p Paths) KeyPath() string  { return filepath.Join(p.DataDir, "agent-key.pem") }

// Cert es el certificado TLS propio del agente, listo para un
// tls.Config.Certificates y con su huella SHA-256 ya en hex — lo que pide
// el contrato ("certSha256" de /info y /pair, TXT mDNS "cert=", y
// `vigia-agent cert`).
type Cert struct {
	TLS tls.Certificate
	// FingerprintSHA256: SHA-256 del certificado (DER), 64 caracteres hex en
	// minúsculas. Mismo valor que calcularía `openssl x509 -fingerprint
	// -sha256` sobre agent-cert.pem (sin los ":"), para poder comprobarlo a
	// mano (ver la prueba real en el encargo).
	FingerprintSHA256 string
}

// LoadOrCreateCert lee el certificado y la llave de dir (ver CertPath/KeyPath
// arriba), o genera un par nuevo si falta cualquiera de los dos archivos —
// un certificado sin su llave (o viceversa) no sirve para nada, así que se
// regeneran juntos. dir debe existir ya con permisos 0700 (igual que
// LoadOrCreateIdentity, ver Paths.EnsureDirs).
func LoadOrCreateCert(dir string) (Cert, error) {
	certPath := filepath.Join(dir, "agent-cert.pem")
	keyPath := filepath.Join(dir, "agent-key.pem")

	certPEM, errCert := os.ReadFile(certPath)
	keyPEM, errKey := os.ReadFile(keyPath)
	switch {
	case errCert == nil && errKey == nil:
		return loadCert(certPEM, keyPEM)
	case errCert != nil && !os.IsNotExist(errCert):
		return Cert{}, fmt.Errorf("leer %s: %w", certPath, errCert)
	case errKey != nil && !os.IsNotExist(errKey):
		return Cert{}, fmt.Errorf("leer %s: %w", keyPath, errKey)
	default:
		return generateAndSaveCert(certPath, keyPath)
	}
}

// RotateCert regenera el certificado y la llave sin importar lo que había
// (ver `vigia-agent cert -rotate`): después de esto, la huella fijada por
// cualquier teléfono ya vinculado deja de coincidir — tienen que
// vincularse de nuevo (lo avisa el CLI, no esta función).
func RotateCert(dir string) (Cert, error) {
	certPath := filepath.Join(dir, "agent-cert.pem")
	keyPath := filepath.Join(dir, "agent-key.pem")
	return generateAndSaveCert(certPath, keyPath)
}

// generateAndSaveCert crea un certificado ECDSA P-256 autofirmado válido 10
// años (ver TLS v1.5: "certificado propio... ECDSA P-256, 10 años") y lo
// guarda junto con su llave en certPath/keyPath con permisos 0600.
func generateAndSaveCert(certPath, keyPath string) (Cert, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Cert{}, fmt.Errorf("generar llave: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return Cert{}, fmt.Errorf("número de serie: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "vigia-agent"},
		// NotBefore con un margen hacia atrás: si el reloj de quien verifica
		// (la app) está un poco atrasado respecto del del host, igual lo
		// acepta — no se valida la cadena, pero Go sí mira las fechas.
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		// Sin SAN a propósito: la app nunca valida el hostname (ver TLS v1.5,
		// "sin validar el hostname: es una IP de la LAN o de Tailscale") — la
		// confianza es por huella, no por nombre.
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return Cert{}, fmt.Errorf("crear certificado: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return Cert{}, fmt.Errorf("serializar llave: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return Cert{}, fmt.Errorf("escribir %s: %w", certPath, err)
	}
	if err := os.Chmod(certPath, 0o600); err != nil {
		return Cert{}, fmt.Errorf("permisos de %s: %w", certPath, err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return Cert{}, fmt.Errorf("escribir %s: %w", keyPath, err)
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		return Cert{}, fmt.Errorf("permisos de %s: %w", keyPath, err)
	}
	return loadCert(certPEM, keyPEM)
}

func loadCert(certPEM, keyPEM []byte) (Cert, error) {
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return Cert{}, fmt.Errorf("cargar certificado/llave: %w", err)
	}
	leaf := tlsCert.Leaf
	if leaf == nil {
		// Go >= 1.23 ya deja Leaf listo (ver tls.X509KeyPair); el parseo
		// aparte es solo por si corriera con un toolchain más viejo.
		parsed, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err != nil {
			return Cert{}, fmt.Errorf("parsear certificado: %w", err)
		}
		leaf = parsed
	}
	return Cert{TLS: tlsCert, FingerprintSHA256: fingerprintHex(leaf)}, nil
}

// fingerprintHex: SHA-256 del DER del certificado, en hex minúsculas (ver
// Cert.FingerprintSHA256).
func fingerprintHex(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}
