package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/internet"
	"github.com/baxner/vigia/agent/store"
)

// fakeInternetProvider es un internetProvider de mentira (ver server.go):
// el estado en vivo lo fija cada test a mano, sin medidor real ni ciclo de
// 30 s — lo que de verdad prueban estos tests es la forma del JSON y el
// 202/409 de POST .../speedtest, no el medidor (eso ya lo prueba
// cmd/vigia-agent).
type fakeInternetProvider struct {
	state         internet.State
	since         time.Time
	now           internet.Sample
	ipv6          internet.IPv6Status
	triggerResult bool
}

func (f *fakeInternetProvider) InternetNow() (internet.State, time.Time, internet.Sample, internet.IPv6Status) {
	return f.state, f.since, f.now, f.ipv6
}

func (f *fakeInternetProvider) TriggerSpeedTest() bool { return f.triggerResult }

// newInternetTestEnv junta un Server real (httptest.NewServer, loopback de
// verdad) con un store en archivo temporal y un token ya vinculado —sin
// pasar por el flujo completo de /api/v1/pair, que no es lo que prueba
// este archivo (ver server_test.go para eso)—.
func newInternetTestEnv(t *testing.T, provider internetProvider) (*httptest.Server, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	const token = "token-de-prueba-internet"
	if _, err := st.CreateClient(context.Background(), "test", store.HashToken(token), time.Now().UTC()); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	srv := NewServer(Deps{
		Store: st, Identity: agent.Identity{ID: "agent-1", Name: "Test"}, Version: "test",
		Hub: NewHub(), Internet: provider,
	})
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return httpSrv, st, token
}

func getJSON(t *testing.T, srv *httptest.Server, token, path string, out any) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return resp
}

func TestHandleInternet_FormaDelContrato(t *testing.T) {
	// Reciente de verdad: handleInternet filtra "outages" contra
	// time.Now() real (últimos 30 días, ver outagesWindow en internet.go),
	// así que una fecha fija del pasado quedaría fuera de la ventana.
	since := time.Now().UTC().Add(-2 * time.Hour)
	provider := &fakeInternetProvider{
		state: internet.StateUp, since: since,
		now: internet.Sample{
			At:       since.Add(time.Minute),
			Router:   &internet.Segment{MS: 2.1, OK: true},
			Modem:    &internet.Segment{MS: 4.3, OK: true},
			Internet: internet.Segment{MS: 18.2, OK: true},
			DNS:      internet.Segment{MS: 12.0, OK: true},
		},
		ipv6: internet.IPv6OK,
	}
	srv, st, token := newInternetTestEnv(t, provider)

	// Una caída ya cerrada y una prueba de velocidad, para que
	// outages/speedTests no salgan vacíos.
	outStart := since.Add(-time.Hour)
	outEnd := outStart.Add(3 * time.Minute)
	id, err := st.InsertOutageOpen(context.Background(), internet.Outage{Start: outStart, Blame: internet.BlameISP})
	if err != nil {
		t.Fatalf("InsertOutageOpen: %v", err)
	}
	if err := st.CloseOutage(context.Background(), id, outEnd); err != nil {
		t.Fatalf("CloseOutage: %v", err)
	}
	if _, err := st.RecordSpeedTest(context.Background(), internet.SpeedTestResult{
		At: since, DownMbps: 95.5, UpMbps: 20.1, LatencyMs: 12, LoadedLatencyMs: 40,
	}); err != nil {
		t.Fatalf("RecordSpeedTest: %v", err)
	}

	var got map[string]any
	resp := getJSON(t, srv, token, "/api/v1/internet", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if got["state"] != "up" {
		t.Fatalf("state = %v, want up", got["state"])
	}
	if got["ipv6"] != "ok" {
		t.Fatalf("ipv6 = %v, want ok", got["ipv6"])
	}
	now, ok := got["now"].(map[string]any)
	if !ok {
		t.Fatalf("now no es un objeto: %v", got["now"])
	}
	if now["routerMs"].(float64) != 2.1 {
		t.Fatalf("now.routerMs = %v, want 2.1", now["routerMs"])
	}
	if now["lossPct"].(float64) != 0 {
		t.Fatalf("now.lossPct = %v, want 0", now["lossPct"])
	}

	outages, ok := got["outages"].([]any)
	if !ok || len(outages) != 1 {
		t.Fatalf("outages = %v, want 1 elemento", got["outages"])
	}
	o := outages[0].(map[string]any)
	if o["blame"] != "isp" {
		t.Fatalf("outages[0].blame = %v, want isp", o["blame"])
	}
	if o["durationSec"].(float64) != 180 {
		t.Fatalf("outages[0].durationSec = %v, want 180", o["durationSec"])
	}
	if _, hasID := o["id"]; hasID {
		t.Fatal("outages[0] no debe traer id (no es un recurso direccionable, ver el contrato)")
	}

	speedTests, ok := got["speedTests"].([]any)
	if !ok || len(speedTests) != 1 {
		t.Fatalf("speedTests = %v, want 1 elemento", got["speedTests"])
	}
	st0 := speedTests[0].(map[string]any)
	if st0["downMbps"].(float64) != 95.5 {
		t.Fatalf("speedTests[0].downMbps = %v, want 95.5", st0["downMbps"])
	}
	if st0["error"] != nil {
		t.Fatalf("speedTests[0].error = %v, want nil", st0["error"])
	}
}

func TestHandleInternet_SinProveedorDa503(t *testing.T) {
	srv, _, token := newInternetTestEnv(t, nil)
	resp := getJSON(t, srv, token, "/api/v1/internet", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

func TestHandleInternetSpeedtest_202Y409(t *testing.T) {
	provider := &fakeInternetProvider{triggerResult: true}
	srv, _, token := newInternetTestEnv(t, provider)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/internet/speedtest", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST speedtest: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}

	provider.triggerResult = false
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST speedtest (2): %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp2.StatusCode)
	}
}

func TestHandleInternetSettings_GetPutValidacion(t *testing.T) {
	srv, _, token := newInternetTestEnv(t, &fakeInternetProvider{})

	var got internet.Settings
	resp := getJSON(t, srv, token, "/api/v1/internet/settings", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d", resp.StatusCode)
	}
	if got != internet.DefaultSettings() {
		t.Fatalf("got = %+v, want default", got)
	}

	// PUT válido.
	body, _ := json.Marshal(internet.Settings{SpeedTestsPerDay: 4})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/internet/settings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", resp2.StatusCode)
	}

	// PUT inválido (fuera de 0..6): 400, sin tocar lo ya guardado.
	body2, _ := json.Marshal(internet.Settings{SpeedTestsPerDay: 99})
	req2, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/internet/settings", bytes.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	resp3, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("PUT inválido: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT inválido status = %d, want 400", resp3.StatusCode)
	}

	var got2 internet.Settings
	getJSON(t, srv, token, "/api/v1/internet/settings", &got2)
	if got2.SpeedTestsPerDay != 4 {
		t.Fatalf("SpeedTestsPerDay = %d, want 4 (el inválido no debió aplicarse)", got2.SpeedTestsPerDay)
	}
}

func TestHandleInternetSeries_RangoInvalido(t *testing.T) {
	srv, _, token := newInternetTestEnv(t, &fakeInternetProvider{})
	resp := getJSON(t, srv, token, "/api/v1/internet/series?range=1y", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandleInternetSeries_RellenaHuecosConNull(t *testing.T) {
	srv, st, token := newInternetTestEnv(t, &fakeInternetProvider{})

	now := time.Now().UTC().Truncate(time.Minute)
	// Solo un minuto con dato, 2 minutos atrás: el resto de la ventana de
	// 24h debe salir con los campos de latencia en null.
	agg := internet.Agg{
		At:       now.Add(-2 * time.Minute),
		Router:   internet.SegmentStats{Sent: 2, Lost: 0, AvgMs: 2},
		Internet: internet.SegmentStats{Sent: 2, Lost: 0, AvgMs: 18},
		DNS:      internet.SegmentStats{Sent: 2, Lost: 0, AvgMs: 10},
	}
	if err := st.RecordMinuteAgg(context.Background(), agg); err != nil {
		t.Fatalf("RecordMinuteAgg: %v", err)
	}

	var got []map[string]any
	resp := getJSON(t, srv, token, "/api/v1/internet/series?range=24h", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(got) == 0 {
		t.Fatal("serie vacía")
	}

	var foundData, foundNull bool
	for _, p := range got {
		if p["routerMs"] == nil {
			foundNull = true
			if p["modemMs"] != nil {
				t.Fatalf("punto sin router pero con modemMs = %v, want null también", p["modemMs"])
			}
		} else if p["routerMs"].(float64) == 2 {
			foundData = true
		}
	}
	if !foundData {
		t.Fatal("no se encontró el punto con dato real (routerMs=2)")
	}
	if !foundNull {
		t.Fatal("no se encontró ningún punto con routerMs=null (hueco sin rellenar)")
	}
}
