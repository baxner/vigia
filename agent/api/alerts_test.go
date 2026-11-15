package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/store"
)

// seedAlert inserta una alerta directo contra el store (sin pasar por
// recordScan: eso no es lo que están probando los tests de este
// archivo) y devuelve la fila persistida.
func (e *testEnv) seedAlert(a alerts.Alert) store.Alert {
	e.t.Helper()
	persisted, created, err := e.store.InsertAlert(context.Background(), a)
	if err != nil || !created {
		e.t.Fatalf("seedAlert: created=%v err=%v", created, err)
	}
	return persisted
}

func TestAlerts_SinTokenDa401(t *testing.T) {
	env := newTestEnv(t)
	resp := env.doJSON(http.MethodGet, "/api/v1/alerts", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAlerts_AfterYLimit(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	var ids []int64
	for i, typ := range []alerts.Type{alerts.AgentBlind, alerts.AgentRecovered, alerts.AgentBlind} {
		a := alerts.Alert{Type: typ, Severity: alerts.Medium, At: t0.Add(time.Duration(i) * time.Minute),
			Title: "t", Body: "b", Network: "net1", DedupKey: "k" + string(rune('A'+i))}
		ids = append(ids, env.seedAlert(a).ID)
	}

	resp := env.doJSON(http.MethodGet, "/api/v1/alerts", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got []store.Alert
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	for i, a := range got {
		if a.ID != ids[i] {
			t.Fatalf("orden = %+v, want ids %v", got, ids)
		}
		if a.Read {
			t.Fatalf("alerta %d: read = true, want false (recién creada)", a.ID)
		}
	}

	resp2 := env.doJSON(http.MethodGet, "/api/v1/alerts?after="+strconv.FormatInt(ids[0], 10)+"&limit=1", token, nil)
	defer resp2.Body.Close()
	var got2 []store.Alert
	json.NewDecoder(resp2.Body).Decode(&got2)
	if len(got2) != 1 || got2[0].ID != ids[1] {
		t.Fatalf("after=%d&limit=1: got %+v, want solo id=%d", ids[0], got2, ids[1])
	}

	resp3 := env.doJSON(http.MethodGet, "/api/v1/alerts?after=abc", token, nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("after inválido: status = %d, want 400", resp3.StatusCode)
	}
}

func TestAlertsRead_MarcaHastaElID(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	a1 := env.seedAlert(alerts.Alert{Type: alerts.AgentBlind, At: t0, Title: "t", Body: "b", DedupKey: "k1"})
	a2 := env.seedAlert(alerts.Alert{Type: alerts.AgentRecovered, At: t0.Add(time.Minute), Title: "t", Body: "b", DedupKey: "k2"})

	resp := env.doJSON(http.MethodPost, "/api/v1/alerts/read", token, alertsReadRequest{UpTo: a1.ID})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	resp2 := env.doJSON(http.MethodGet, "/api/v1/alerts", token, nil)
	defer resp2.Body.Close()
	var got []store.Alert
	json.NewDecoder(resp2.Body).Decode(&got)
	byID := map[int64]store.Alert{}
	for _, a := range got {
		byID[a.ID] = a
	}
	if !byID[a1.ID].Read {
		t.Fatal("a1.Read = false, want true")
	}
	if byID[a2.ID].Read {
		t.Fatal("a2.Read = true, want false (upTo no lo alcanza)")
	}
}

func TestAlertSettings_GetPutRoundTrip(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	resp := env.doJSON(http.MethodGet, "/api/v1/alert-settings", token, nil)
	defer resp.Body.Close()
	var got alerts.Settings
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != alerts.DefaultSettings() {
		t.Fatalf("GET inicial = %+v, want todo encendido", got)
	}

	nuevo := alerts.Settings{NewDevice: false, ImportantOffline: true, ImportantOnline: false, AgentBlind: true, AgentRecovered: false}
	resp2 := env.doJSON(http.MethodPut, "/api/v1/alert-settings", token, nuevo)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", resp2.StatusCode)
	}
	var putResp alerts.Settings
	json.NewDecoder(resp2.Body).Decode(&putResp)
	if putResp != nuevo {
		t.Fatalf("PUT devolvió %+v, want %+v", putResp, nuevo)
	}

	resp3 := env.doJSON(http.MethodGet, "/api/v1/alert-settings", token, nil)
	defer resp3.Body.Close()
	var got3 alerts.Settings
	json.NewDecoder(resp3.Body).Decode(&got3)
	if got3 != nuevo {
		t.Fatalf("GET tras PUT = %+v, want %+v", got3, nuevo)
	}
}

func TestStream_RecibeEventoAlert(t *testing.T) {
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

	alert := store.Alert{ID: 7, Type: alerts.NewDevice, Severity: alerts.High, At: time.Now().UTC(),
		Title: "Equipo nuevo en tu red", Body: "X (1.2.3.4) se conectó por primera vez."}

	reader := bufio.NewReader(resp.Body)
	var gotEvent, gotData string
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		env.hub.PublishAlert(alert)

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
			if gotEvent == "alert" && gotData != "" {
				goto done
			}
		}
	}
done:
	if gotEvent != "alert" {
		t.Fatalf("event = %q, want alert", gotEvent)
	}
	var decoded store.Alert
	if err := json.Unmarshal([]byte(gotData), &decoded); err != nil {
		t.Fatalf("decode data: %v (data=%q)", err, gotData)
	}
	if decoded.ID != 7 || decoded.Title != alert.Title {
		t.Fatalf("decoded = %+v, want id=7 title=%q", decoded, alert.Title)
	}
}
