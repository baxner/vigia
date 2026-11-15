package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/internet"
	"github.com/baxner/vigia/agent/store"
)

func TestPairRateLimit_BloqueaDespuesDe10PorIP(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	srv := NewServer(Deps{Store: st, Identity: agent.Identity{ID: "a1", Name: "Node"}})
	srv.pairLimiter.now = func() time.Time { return now }

	doPairFrom := func(remoteAddr string) int {
		body, _ := json.Marshal(pairRequest{Code: "000000", Client: "Tester"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pair", bytes.NewReader(body))
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < pairRateLimitMax; i++ {
		if code := doPairFrom("192.168.1.50:12345"); code != http.StatusUnauthorized {
			t.Fatalf("intento %d: status = %d, want 401", i+1, code)
		}
	}
	if code := doPairFrom("192.168.1.50:12346"); code != http.StatusTooManyRequests {
		t.Fatalf("intento 11 misma IP: status = %d, want 429", code)
	}

	// Otra IP sigue pudiendo intentar.
	if code := doPairFrom("192.168.1.51:22222"); code != http.StatusUnauthorized {
		t.Fatalf("otra IP: status = %d, want 401", code)
	}

	// Pasado 1 minuto vuelve a permitir la primera IP.
	now = now.Add(time.Minute + time.Second)
	if code := doPairFrom("192.168.1.50:12347"); code != http.StatusUnauthorized {
		t.Fatalf("tras ventana de 1m: status = %d, want 401", code)
	}
}

func TestNetworks_VacioDevuelveArrayJSON(t *testing.T) {
	env := newTestEnv(t)
	token := env.pair("Tester")

	resp := env.doJSON(http.MethodGet, "/api/v1/networks", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw := strings.TrimSpace(readAllString(t, resp.Body))
	if raw != "[]" {
		t.Fatalf("body = %q, want [] (no null)", raw)
	}
}

func TestHubSubscribeBuffer128(t *testing.T) {
	h := NewHub()
	ch := h.subscribe()
	defer h.unsubscribe(ch)
	if cap(ch) != 128 {
		t.Fatalf("cap(ch) = %d, want 128", cap(ch))
	}
}

func TestHandleInternetSeries_IncluyeBucketDelBordeSince(t *testing.T) {
	srv, st, token := newInternetTestEnv(t, &fakeInternetProvider{})

	// Punto exacto en el bucket inicial de la ventana de 24h:
	// now.Add(-24h).Truncate(time.Minute). Con la consulta anterior sin
	// truncar, un "At" en :00 segundos quedaba fuera cuando now tenía
	// segundos > 0 (o quedaba al borde de perder el primer punto).
	edgeBucket := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Minute)
	if err := st.RecordMinuteAgg(context.Background(), internet.Agg{
		At:       edgeBucket,
		Router:   internet.SegmentStats{Sent: 2, Lost: 0, AvgMs: 3.5},
		Internet: internet.SegmentStats{Sent: 2, Lost: 0, AvgMs: 20},
		DNS:      internet.SegmentStats{Sent: 2, Lost: 0, AvgMs: 11},
	}); err != nil {
		t.Fatalf("RecordMinuteAgg: %v", err)
	}

	var got []map[string]any
	resp := getJSON(t, srv, token, "/api/v1/internet/series?range=24h", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(got) == 0 || got[0]["routerMs"] == nil || got[0]["routerMs"].(float64) != 3.5 {
		t.Fatalf("got[0] = %+v, want routerMs=3.5 en el primer bucket %s", got[0], edgeBucket)
	}
}
