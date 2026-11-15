package api

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/store"
)

// TestInfo_CertSha256_SeDevuelveTalCual: /api/v1/info manda exactamente lo
// que trae Deps.CertSHA256 (ver TLS v1.5, docs/agente-api.md) — el cableado
// con el certificado de verdad lo prueba TestHTTPSServer_LaHuellaDelCertificadoServidoCoincideConInfo,
// más abajo, contra un servidor TLS real.
func TestInfo_CertSha256_SeDevuelveTalCual(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	const wantFingerprint = "deadbeef00112233445566778899aabbccddeeff00112233445566778899aa"
	srv := NewServer(Deps{Store: st, Identity: agent.Identity{ID: "x", Name: "x"}, CertSHA256: wantFingerprint})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var info infoResponse
	if err := json.NewDecoder(rec.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.CertSHA256 != wantFingerprint {
		t.Fatalf("certSha256 = %q, want %q", info.CertSHA256, wantFingerprint)
	}
}

// TestPair_CertSha256_SeDevuelveTalCual: la respuesta de POST /api/v1/pair
// también trae la huella (ver el contrato: "la respuesta de /pair incluyen
// certSha256"), para que la app la fije de entrada, sin necesitar un
// segundo viaje a /info.
func TestPair_CertSha256_SeDevuelveTalCual(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	const wantFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"
	identity := agent.Identity{ID: "agent-1", Name: "Agente-de-Prueba"}
	srv := NewServer(Deps{Store: st, Identity: identity, CertSHA256: wantFingerprint})
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	env := &testEnv{t: t, server: httpSrv, store: st}
	token := env.pair("Galaxy S24 Ultra")
	if token == "" {
		t.Fatal("pair() no devolvió token")
	}

	// env.pair ya comprobó el token; releemos /pair a mano para inspeccionar
	// certSha256, que pair() (helper compartido con otras pruebas) no expone.
	resp := env.doJSON(http.MethodPost, "/internal/pair-code", "", nil)
	defer resp.Body.Close()
	var code pairCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&code); err != nil {
		t.Fatalf("decode pair-code: %v", err)
	}
	resp2 := env.doJSON(http.MethodPost, "/api/v1/pair", "", pairRequest{Code: code.Code, Client: "Otro"})
	defer resp2.Body.Close()
	var pr pairResponse
	if err := json.NewDecoder(resp2.Body).Decode(&pr); err != nil {
		t.Fatalf("decode pair response: %v", err)
	}
	if pr.CertSHA256 != wantFingerprint {
		t.Fatalf("certSha256 = %q, want %q", pr.CertSHA256, wantFingerprint)
	}
}

// TestHTTPSServer_LaHuellaDelCertificadoServidoCoincideConInfo monta un
// *Server de verdad detrás de un servidor HTTPS (httptest.NewUnstartedServer
// + StartTLS, ver el encargo: "servidor HTTPS con httptest") usando el
// certificado real de agent.LoadOrCreateCert — el mismo que generaría
// `vigia-agent run`. Comprueba de punta a punta: el certificado que el
// servidor PRESENTA en el handshake tiene la misma huella SHA-256 que
// agent.Cert.FingerprintSHA256 y que el "certSha256" que /api/v1/info
// reporta en el cuerpo — las tres cuentas que la app (y `vigia-agent cert`)
// tienen que poder comparar entre sí.
func TestHTTPSServer_LaHuellaDelCertificadoServidoCoincideConInfo(t *testing.T) {
	dir := t.TempDir()
	cert, err := agent.LoadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCert: %v", err)
	}

	st, err := store.Open(context.Background(), filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	srv := NewServer(Deps{Store: st, Identity: agent.Identity{ID: "x", Name: "x"}, CertSHA256: cert.FingerprintSHA256})

	ts := httptest.NewUnstartedServer(srv)
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{cert.TLS}}
	ts.StartTLS()
	defer ts.Close()

	// Cliente que no valida nada (como el curl -k de la prueba real): lo que
	// importa es leer el certificado presentado, no confiar en él a ciegas.
	var presentedFingerprint string
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // es justo lo que se está inspeccionando
				VerifyConnection: func(cs tls.ConnectionState) error {
					if len(cs.PeerCertificates) == 0 {
						return nil
					}
					sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
					presentedFingerprint = hex.EncodeToString(sum[:])
					return nil
				},
			},
		},
	}

	resp, err := client.Get(ts.URL + "/api/v1/info")
	if err != nil {
		t.Fatalf("GET /api/v1/info por HTTPS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var info infoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if presentedFingerprint == "" {
		t.Fatal("no se capturó el certificado presentado en el handshake")
	}
	if presentedFingerprint != cert.FingerprintSHA256 {
		t.Fatalf("huella presentada = %q, want %q (la de agent.LoadOrCreateCert)", presentedFingerprint, cert.FingerprintSHA256)
	}
	if info.CertSHA256 != cert.FingerprintSHA256 {
		t.Fatalf("certSha256 del cuerpo = %q, want %q", info.CertSHA256, cert.FingerprintSHA256)
	}
}
