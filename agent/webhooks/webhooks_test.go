package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"
	"time"
)

func TestSign_HMACSHA256Hex(t *testing.T) {
	secret, body := "shh", []byte(`{"a":1}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	if got := Sign(secret, body); got != want {
		t.Fatalf("Sign = %q, want %q", got, want)
	}
	// Firmas distintas para cuerpos distintos (si no, de nada serviría).
	if Sign(secret, []byte(`{"a":2}`)) == want {
		t.Fatal("cuerpos distintos produjeron la misma firma")
	}
}

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewDeliveryID_FormatoUUIDv4YDistintos(t *testing.T) {
	a, err := NewDeliveryID()
	if err != nil {
		t.Fatalf("NewDeliveryID: %v", err)
	}
	if !uuidV4.MatchString(a) {
		t.Fatalf("NewDeliveryID = %q, no tiene forma de UUID v4", a)
	}
	b, _ := NewDeliveryID()
	if a == b {
		t.Fatal("dos llamadas devolvieron el mismo id")
	}
}

func TestResult_Success(t *testing.T) {
	status := func(n int) Result { return Result{Status: &n} }
	cases := []struct {
		r    Result
		want bool
	}{
		{Result{}, false},
		{status(199), false},
		{status(200), true},
		{status(299), true},
		{status(300), false},
		{status(404), false},
		{status(500), false},
	}
	for _, c := range cases {
		if got := c.r.Success(); got != c.want {
			t.Fatalf("Success(%+v) = %v, want %v", c.r, got, c.want)
		}
	}
}

func TestAttempt_MandaCabecerasYFirmaVerificable(t *testing.T) {
	secret := "mi-secreto"
	var gotBody []byte
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSender("1.2.3")
	s.allowLoopbackForTest = true
	body := []byte(`{"event":"alert"}`)
	status, err := s.Attempt(context.Background(), Target{ID: 1, URL: srv.URL, Secret: secret}, body, "NEW_DEVICE", "delivery-123")
	if err != nil {
		t.Fatalf("Attempt: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if string(gotBody) != string(body) {
		t.Fatalf("body recibido = %q, want %q", gotBody, body)
	}
	if ua := gotHeaders.Get("User-Agent"); ua != "Vigia-Agent/1.2.3" {
		t.Fatalf("User-Agent = %q, want Vigia-Agent/1.2.3", ua)
	}
	if ev := gotHeaders.Get("X-Vigia-Event"); ev != "NEW_DEVICE" {
		t.Fatalf("X-Vigia-Event = %q, want NEW_DEVICE", ev)
	}
	if d := gotHeaders.Get("X-Vigia-Delivery"); d != "delivery-123" {
		t.Fatalf("X-Vigia-Delivery = %q, want delivery-123", d)
	}
	wantSig := "sha256=" + Sign(secret, gotBody)
	if sig := gotHeaders.Get("X-Vigia-Signature"); sig != wantSig {
		t.Fatalf("X-Vigia-Signature = %q, want %q (verificada con el cuerpo que de verdad llegó)", sig, wantSig)
	}
}

func TestAttempt_SinSecreto_NoMandaFirma(t *testing.T) {
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSender("test")
	s.allowLoopbackForTest = true
	if _, err := s.Attempt(context.Background(), Target{URL: srv.URL}, []byte("{}"), "NEW_DEVICE", "d1"); err != nil {
		t.Fatalf("Attempt: %v", err)
	}
	if sig := gotHeaders.Get("X-Vigia-Signature"); sig != "" {
		t.Fatalf("X-Vigia-Signature = %q, want vacío sin secreto", sig)
	}
}

func TestAttempt_RechazaURLNoHTTP(t *testing.T) {
	s := NewSender("test")
	s.allowLoopbackForTest = true
	for _, url := range []string{"ftp://example.com/x", "file:///etc/passwd", "javascript:alert(1)", ""} {
		if _, err := s.Attempt(context.Background(), Target{URL: url}, []byte("{}"), "NEW_DEVICE", "d1"); err == nil {
			t.Fatalf("url=%q: esperaba un error (solo http/https)", url)
		}
	}
}

func TestDeliver_ReintentaHastaExito(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := &Sender{Version: "test", Backoff: []time.Duration{time.Millisecond, time.Millisecond}, allowLoopbackForTest: true}
	res := s.Deliver(context.Background(), Target{URL: srv.URL}, []byte("{}"), "NEW_DEVICE")
	if !res.Success() {
		t.Fatalf("res = %+v, want éxito tras reintentar", res)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("hits = %d, want 3 (2 fallos + el que tuvo éxito)", got)
	}
}

func TestDeliver_AgotaIntentosYDevuelveUltimoEstado(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	backoff := []time.Duration{time.Millisecond, time.Millisecond}
	s := &Sender{Version: "test", Backoff: backoff, allowLoopbackForTest: true}
	res := s.Deliver(context.Background(), Target{URL: srv.URL}, []byte("{}"), "NEW_DEVICE")
	if res.Success() {
		t.Fatalf("res = %+v, want fallo (siempre 503)", res)
	}
	if res.Status == nil || *res.Status != http.StatusServiceUnavailable {
		t.Fatalf("Status = %v, want 503 (el último que respondió el servidor)", res.Status)
	}
	wantAttempts := int32(len(backoff) + 1)
	if got := hits.Load(); got != wantAttempts {
		t.Fatalf("hits = %d, want %d (1 + len(backoff) reintentos)", got, wantAttempts)
	}
}

func TestDeliver_NoSigueRedirecciones(t *testing.T) {
	var redirectHits atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	defer srv.Close()

	s := &Sender{Version: "test", Backoff: []time.Duration{time.Millisecond}, allowLoopbackForTest: true}
	res := s.Deliver(context.Background(), Target{URL: srv.URL}, []byte("{}"), "NEW_DEVICE")
	if res.Status == nil || *res.Status != http.StatusFound {
		t.Fatalf("Status = %v, want 302 (la respuesta de redirección misma, sin seguirla)", res.Status)
	}
	if got := redirectHits.Load(); got != 0 {
		t.Fatalf("redirectHits = %d, want 0: el agente nunca debe seguir la redirección", got)
	}
	if got := hits.Load(); got != int32(len([]time.Duration{time.Millisecond})+1) {
		t.Fatalf("hits = %d, want 2 (reintenta porque 302 no es éxito)", got)
	}
}

func TestDeliver_ContextCancelado_NoEsperaElBackoffCompleto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &Sender{Version: "test", Backoff: []time.Duration{10 * time.Second}, allowLoopbackForTest: true}

	done := make(chan Result, 1)
	go func() { done <- s.Deliver(ctx, Target{URL: srv.URL}, []byte("{}"), "NEW_DEVICE") }()

	time.Sleep(20 * time.Millisecond) // deja que el primer intento (fallido) ya haya pasado
	cancel()

	select {
	case res := <-done:
		if res.Success() {
			t.Fatalf("res = %+v, want fallo", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Deliver no respetó la cancelación del contexto: siguió esperando el backoff completo de 10s")
	}
}

func TestDeliver_TargetInvalido_NoIntentaReintentar(t *testing.T) {
	s := &Sender{Version: "test", Backoff: []time.Duration{time.Millisecond, time.Millisecond}, allowLoopbackForTest: true}
	res := s.Deliver(context.Background(), Target{URL: "ftp://x"}, []byte("{}"), "NEW_DEVICE")
	if res.Success() || res.Status != nil {
		t.Fatalf("res = %+v, want solo un error de transporte", res)
	}
	if res.Err == nil {
		t.Fatal("Err = nil, want un error (url no http/https)")
	}
}
