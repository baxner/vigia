package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/core"
)

// fakeScheduler: un scanTrigger de mentira, para probar el 202/409 de
// POST /api/v1/scan sin depender del tiempo real de un core.Scan de verdad
// (eso ya lo prueba agent/scheduler).
type fakeScheduler struct {
	mu       sync.Mutex
	busy     bool
	lastScan time.Time
	interval time.Duration
}

func (f *fakeScheduler) TriggerNow() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy {
		return false
	}
	f.busy = true
	return true
}

func (f *fakeScheduler) release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.busy = false
}

func (f *fakeScheduler) LastScanAt() time.Time   { f.mu.Lock(); defer f.mu.Unlock(); return f.lastScan }
func (f *fakeScheduler) Interval() time.Duration { return f.interval }

// testEnv junta un Server real detrás de un httptest.NewServer (TCP local
// de verdad: así las conexiones llegan genuinamente desde loopback, que es
// justo lo que exige /internal/pair-code) con un store en archivo temporal.
type testEnv struct {
	t        *testing.T
	server   *httptest.Server
	store    *store.Store
	hub      *Hub
	sched    *fakeScheduler
	identity agent.Identity
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	hub := NewHub()
	sched := &fakeScheduler{interval: 5 * time.Minute}
	identity := agent.Identity{ID: "agent-123", Name: "Agente-de-Prueba"}

	srv := NewServer(Deps{Store: st, Scheduler: sched, Identity: identity, Version: "test", Hub: hub})
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)

	return &testEnv{t: t, server: httpSrv, store: st, hub: hub, sched: sched, identity: identity}
}

func (e *testEnv) url(path string) string { return e.server.URL + path }

func (e *testEnv) doJSON(method, path, token string, body any) *http.Response {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, e.url(path), reader)
	if err != nil {
		e.t.Fatalf("NewRequest %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// pair hace el flujo de vinculación completo por HTTP real (loopback de
// verdad, nada que falsear) y devuelve el token emitido.
func (e *testEnv) pair(client string) string {
	e.t.Helper()
	resp := e.doJSON(http.MethodPost, "/internal/pair-code", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("/internal/pair-code: status=%d", resp.StatusCode)
	}
	var code pairCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&code); err != nil {
		e.t.Fatalf("decode pair-code: %v", err)
	}

	resp2 := e.doJSON(http.MethodPost, "/api/v1/pair", "", pairRequest{Code: code.Code, Client: client})
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		e.t.Fatalf("/api/v1/pair: status=%d", resp2.StatusCode)
	}
	var pr pairResponse
	if err := json.NewDecoder(resp2.Body).Decode(&pr); err != nil {
		e.t.Fatalf("decode pair response: %v", err)
	}
	if pr.Token == "" {
		e.t.Fatal("token vacío")
	}
	return pr.Token
}

// seedDevice registra un escaneo directo contra el store (sin pasar por el
// scheduler: eso no es lo que están probando los tests de este archivo) y
// devuelve el KnownDevice resultante.
func (e *testEnv) seedDevice(net store.NetworkInfo, dev core.Device, at time.Time) history.KnownDevice {
	e.t.Helper()
	if _, err := e.store.RecordScan(context.Background(), net, []core.Device{dev}, at); err != nil {
		e.t.Fatalf("RecordScan: %v", err)
	}
	known, err := e.store.DevicesByNetwork(context.Background(), net.Key)
	if err != nil || len(known) == 0 {
		e.t.Fatalf("DevicesByNetwork: known=%v err=%v", known, err)
	}
	return known[len(known)-1]
}

var testNet = store.NetworkInfo{
	Key:     history.NewNetworkKey("192.168.0.1", "192.168.0.0/24"),
	Subnet:  "192.168.0.0/24",
	Gateway: "192.168.0.1",
}

func TestInfo_SinToken(t *testing.T) {
	env := newTestEnv(t)

	resp := env.doJSON(http.MethodGet, "/api/v1/info", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var info infoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.ID != env.identity.ID || info.Name != env.identity.Name {
		t.Fatalf("info = %+v, want id=%q name=%q", info, env.identity.ID, env.identity.Name)
	}
	if info.ScanIntervalSec != 300 {
		t.Fatalf("scanIntervalSec = %d, want 300", info.ScanIntervalSec)
	}
	if info.LastScanAt != nil {
		t.Fatalf("lastScanAt = %v, want nil (todavía no escaneó)", *info.LastScanAt)
	}
}

// Addresses es inyectable (como Warning): /api/v1/info debe servir
// exactamente lo que da esa función, sin tocarlo (ver docs/agente-api.md,
// v1.1: "addresses" incluye la IP de Tailscale si la hay, pero eso lo
// decide agent.LocalIPv4Addresses — este test solo prueba el cableado).
func TestInfo_AddressesInyectadasTalCual(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	want := []string{"100.64.0.10", "192.168.0.48"}
	srv := NewServer(Deps{Store: st, Identity: agent.Identity{ID: "x", Name: "x"},
		Addresses: func() []string { return want }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var info infoResponse
	if err := json.NewDecoder(rec.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(info.Addresses) != 2 || info.Addresses[0] != want[0] || info.Addresses[1] != want[1] {
		t.Fatalf("addresses = %v, want %v", info.Addresses, want)
	}
}

// Sin override, /api/v1/info nunca debe devolver "addresses": null —
// agent.LocalIPv4Addresses ya garantiza no-nil, pero esto prueba el
// cableado del valor por defecto en NewServer.
func TestInfo_AddressesPorDefectoNuncaNull(t *testing.T) {
	env := newTestEnv(t)
	resp := env.doJSON(http.MethodGet, "/api/v1/info", "", nil)
	defer resp.Body.Close()
	var info infoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Addresses == nil {
		t.Fatal("addresses = null, want [] como mínimo")
	}
}

func TestInternalPairCode_RechazaSiNoVieneDeLoopback(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	srv := NewServer(Deps{Store: st, Identity: agent.Identity{ID: "x", Name: "x"}})

	req := httptest.NewRequest(http.MethodPost, "/internal/pair-code", nil)
	req.RemoteAddr = "203.0.113.5:54321" // no es loopback
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestPair_CicloCompletoYTokenSirveParaAutenticar(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Galaxy S24 Ultra")

	resp := env.doJSON(http.MethodGet, "/api/v1/networks", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /networks con token recién vinculado: status=%d", resp.StatusCode)
	}

	clients, err := env.store.Clients(context.Background())
	if err != nil || len(clients) != 1 || clients[0].Name != "Galaxy S24 Ultra" {
		t.Fatalf("Clients() = %+v err=%v, want 1 cliente \"Galaxy S24 Ultra\"", clients, err)
	}
}

func TestPair_CodigoEquivocadoDa401(t *testing.T) {
	env := newTestEnv(t)
	resp := env.doJSON(http.MethodPost, "/api/v1/pair", "", pairRequest{Code: "000000", Client: "X"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	var e errorResponse
	json.NewDecoder(resp.Body).Decode(&e)
	if e.Error == "" {
		t.Fatal("esperaba un mensaje de error")
	}
}

func TestPair_CamposObligatorios(t *testing.T) {
	env := newTestEnv(t)
	resp := env.doJSON(http.MethodPost, "/api/v1/pair", "", pairRequest{Code: "", Client: "X"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAuth_FaltaTokenORechazaTokenInvalido(t *testing.T) {
	env := newTestEnv(t)

	resp := env.doJSON(http.MethodGet, "/api/v1/networks", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sin token: status = %d, want 401", resp.StatusCode)
	}

	resp2 := env.doJSON(http.MethodGet, "/api/v1/networks", "un-token-que-no-existe", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token inválido: status = %d, want 401", resp2.StatusCode)
	}
}

func TestNetworksYDevices_ListanLoQueHayEnElStore(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	env.seedDevice(testNet, core.Device{IP: "192.168.0.10", MAC: "aa:bb:cc:dd:ee:01"}, time.Now().UTC())

	resp := env.doJSON(http.MethodGet, "/api/v1/networks", token, nil)
	defer resp.Body.Close()
	var nets []store.Network
	if err := json.NewDecoder(resp.Body).Decode(&nets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(nets) != 1 || nets[0].Key != string(testNet.Key) {
		t.Fatalf("nets = %+v, want 1 con key=%q", nets, testNet.Key)
	}

	// {key} va URL-encoded: la subred trae un "/" (CIDR) y además hay un "|".
	encodedKey := url.PathEscape(string(testNet.Key))
	resp2 := env.doJSON(http.MethodGet, "/api/v1/networks/"+encodedKey+"/devices", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp2.StatusCode)
	}
	var devices []history.KnownDevice
	if err := json.NewDecoder(resp2.Body).Decode(&devices); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(devices) != 1 || devices[0].LastIP != "192.168.0.10" {
		t.Fatalf("devices = %+v", devices)
	}
	if devices[0].MAC == nil || *devices[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("mac = %v, want aa:bb:cc:dd:ee:01", devices[0].MAC)
	}
}

func TestNetworkDevices_RedDesconocidaDaListaVaciaNo404(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	resp := env.doJSON(http.MethodGet, "/api/v1/networks/"+url.PathEscape("x|y")+"/devices", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var devices []history.KnownDevice
	json.NewDecoder(resp.Body).Decode(&devices)
	if len(devices) != 0 {
		t.Fatalf("devices = %+v, want vacío", devices)
	}
}

func TestPatchDevice_SemanticaAusenteVsNull(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	d := env.seedDevice(testNet, core.Device{IP: "192.168.0.10", MAC: "aa:bb:cc:dd:ee:01"}, time.Now().UTC())
	path := fmt.Sprintf("/api/v1/devices/%d", d.ID)

	resp := env.doJSON(http.MethodPatch, path, token, map[string]any{"name": "Laptop", "important": true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var updated history.KnownDevice
	json.NewDecoder(resp.Body).Decode(&updated)
	if updated.Custom.Name == nil || *updated.Custom.Name != "Laptop" {
		t.Fatalf("name = %v, want Laptop", updated.Custom.Name)
	}
	if !updated.Custom.Important {
		t.Fatal("important = false, want true")
	}

	// Campo ausente (location): no toca lo ya fijado. typeId con null explícito: lo deja en nil.
	resp2 := env.doJSON(http.MethodPatch, path, token, map[string]any{"typeId": nil})
	defer resp2.Body.Close()
	var updated2 history.KnownDevice
	json.NewDecoder(resp2.Body).Decode(&updated2)
	if updated2.Custom.Name == nil || *updated2.Custom.Name != "Laptop" {
		t.Fatalf("un PATCH de otro campo no debería borrar name, got %v", updated2.Custom.Name)
	}
	if updated2.Custom.TypeID != nil {
		t.Fatalf("typeId = %v, want nil tras mandar null", updated2.Custom.TypeID)
	}

	// important no admite null (no es nullable en el contrato).
	resp3 := env.doJSON(http.MethodPatch, path, token, map[string]any{"important": nil})
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("important:null status = %d, want 400", resp3.StatusCode)
	}

	resp4 := env.doJSON(http.MethodPatch, "/api/v1/devices/999999", token, map[string]any{"name": "X"})
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusNotFound {
		t.Fatalf("id inexistente: status = %d, want 404", resp4.StatusCode)
	}
}

func TestDeleteDevice(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	d := env.seedDevice(testNet, core.Device{IP: "192.168.0.10", MAC: "aa:bb:cc:dd:ee:01"}, time.Now().UTC())
	path := fmt.Sprintf("/api/v1/devices/%d", d.ID)

	resp := env.doJSON(http.MethodDelete, path, token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	resp2 := env.doJSON(http.MethodGet, path, token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("tras borrar: status = %d, want 404", resp2.StatusCode)
	}

	resp3 := env.doJSON(http.MethodDelete, path, token, nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("borrar de nuevo: status = %d, want 404", resp3.StatusCode)
	}
}

func TestDeviceEvents_OrdenYLimite(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	mac := "aa:bb:cc:dd:ee:01"
	t0 := time.Now().UTC().Add(-time.Hour)
	d := env.seedDevice(testNet, core.Device{IP: "192.168.0.10", MAC: mac}, t0)

	if _, err := env.store.RecordScan(context.Background(), testNet, []core.Device{{IP: "192.168.0.11", MAC: mac}}, t0.Add(time.Minute)); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}

	resp := env.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/events?limit=1", d.ID), token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var events []history.DeviceEvent
	json.NewDecoder(resp.Body).Decode(&events)
	if len(events) != 1 || events[0].Type != history.IPChanged {
		t.Fatalf("events = %+v, want 1 evento IP_CHANGED (limit=1, el más reciente)", events)
	}

	resp2 := env.doJSON(http.MethodGet, "/api/v1/devices/999999/events", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("id inexistente: status = %d, want 404", resp2.StatusCode)
	}
}

func TestScan_202YLuego409YLuego202DeNuevo(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	resp := env.doJSON(http.MethodPost, "/api/v1/scan", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}

	resp2 := env.doJSON(http.MethodPost, "/api/v1/scan", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (ya hay uno en curso)", resp2.StatusCode)
	}

	env.sched.release()
	resp3 := env.doJSON(http.MethodPost, "/api/v1/scan", token, nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusAccepted {
		t.Fatalf("tras liberar: status = %d, want 202", resp3.StatusCode)
	}
}

func TestStream_RecibeEventosPublicadosPorElHub(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.url("/api/v1/stream"), nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Publicar justo al conectar puede ganarle al subscribe(); reintentar
	// un par de veces es más robusto que un sleep fijo.
	reader := bufio.NewReader(resp.Body)
	var gotEvent, gotData string
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		env.hub.PublishScan(string(testNet.Key), time.Now(), time.Now(), 3)

		readDeadline := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(readDeadline) {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event:"):
				gotEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				gotData = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
			if gotEvent == "scan" && gotData != "" {
				goto done
			}
		}
	}
done:
	if gotEvent != "scan" {
		t.Fatalf("event = %q, want scan", gotEvent)
	}
	if !strings.Contains(gotData, string(testNet.Key)) {
		t.Fatalf("data = %q, want contener %q", gotData, testNet.Key)
	}
}
