package agent

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// startDual levanta un http.Server real sobre NewDualListener en un puerto
// libre y devuelve su dirección (127.0.0.1:N).
func startDual(t *testing.T, cert Cert, allowPlain bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "hola")
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go srv.Serve(NewDualListener(ln, &tls.Config{Certificates: []tls.Certificate{cert.TLS}}, allowPlain))
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func get(t *testing.T, client *http.Client, url string) (string, error) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func TestDualListener_HTTPSyHTTPEnElMismoPuerto(t *testing.T) {
	cert, err := LoadOrCreateCert(t.TempDir())
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	addr := startDual(t, cert, true)

	var seen string
	tlsClient := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
			seen = hex.EncodeToString(sum[:])
			return nil
		},
	}}}
	if body, err := get(t, tlsClient, "https://"+addr+"/"); err != nil || body != "hola" {
		t.Fatalf("HTTPS: body=%q err=%v", body, err)
	}
	if seen != cert.FingerprintSHA256 {
		t.Fatalf("huella servida = %s, want %s", seen, cert.FingerprintSHA256)
	}

	plain := &http.Client{Timeout: 5 * time.Second}
	if body, err := get(t, plain, "http://"+addr+"/"); err != nil || body != "hola" {
		t.Fatalf("HTTP: body=%q err=%v", body, err)
	}
}

func TestDualListener_SinLegadoRechazaHTTP(t *testing.T) {
	cert, err := LoadOrCreateCert(t.TempDir())
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	addr := startDual(t, cert, false)

	plain := &http.Client{Timeout: 5 * time.Second}
	if _, err := get(t, plain, "http://"+addr+"/"); err == nil {
		t.Fatal("HTTP sin cifrar contestó con -http-legacy=false")
	}
	tlsClient := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	if body, err := get(t, tlsClient, "https://"+addr+"/"); err != nil || body != "hola" {
		t.Fatalf("HTTPS: body=%q err=%v", body, err)
	}
}
