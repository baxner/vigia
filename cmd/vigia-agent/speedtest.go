package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/baxner/vigia/agent/internet"
	"github.com/baxner/vigia/agent/store"
)

// cloudflareSpeedTestBaseURL: el servidor de verdad (ver
// docs/agente-api.md, "Prueba de velocidad"). Parámetro (no una constante
// usada directo) para que las pruebas puedan apuntar a un httptest.Server
// falso en vez de salir a internet (ver speedtest_test.go).
const cloudflareSpeedTestBaseURL = "https://speed.cloudflare.com"

// cloudflareDownloadBytes/UploadBytes: ver docs/agente-api.md: "~25 MB de
// bajada... y ~10 MB de subida".
const (
	cloudflareDownloadBytes = 25_000_000
	cloudflareUploadBytes   = 10_000_000
)

// cloudflareTransferBudget: tope de tiempo por sentido de la prueba (ver
// docs/plan.md, Fase 7: "o varios tramos hasta ~10 s"). No se espera a
// terminar de bajar/subir cloudflareDownloadBytes/UploadBytes completos si
// eso tardara más: se corta a este tiempo y se calcula Mbps con lo que de
// verdad se transfirió — así una conexión lenta no alarga la prueba de
// más, y una rápida no se queda corta de muestra.
const cloudflareTransferBudget = 10 * time.Second

// cloudflareTotalBudget: tope de TODA la prueba (latencia + bajada +
// subida), ver docs/agente-api.md: "Timeouts y tope de bytes para no
// pasarse".
const cloudflareTotalBudget = 30 * time.Second

// msOfSeconds: milisegundos a partir de una duración en segundos (float),
// para redactar LatencyMs/LoadedLatencyMs.
func msOfSeconds(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// speedTestFunc corre UNA prueba de velocidad completa. Inyectable (ver
// newSpeedTestRunner) para poder probar speedTestRunner sin red real.
type speedTestFunc func(ctx context.Context) internet.SpeedTestResult

// newCloudflareSpeedTest arma el speedTestFunc de verdad contra baseURL
// (speed.cloudflare.com en producción; la URL de un httptest.Server en las
// pruebas). client es compartido entre llamadas (reutiliza conexiones).
func newCloudflareSpeedTest(client *http.Client, baseURL string) speedTestFunc {
	return func(ctx context.Context) internet.SpeedTestResult {
		at := time.Now().UTC()
		ctx, cancel := context.WithTimeout(ctx, cloudflareTotalBudget)
		defer cancel()

		idleLatency, err := cloudflareLatency(ctx, client, baseURL)
		if err != nil {
			return speedTestError(at, err)
		}
		downMbps, loadedLatency, err := cloudflareDownloadWithLatency(ctx, client, baseURL)
		if err != nil {
			return speedTestError(at, err)
		}
		upMbps, err := cloudflareUpload(ctx, client, baseURL)
		if err != nil {
			return speedTestError(at, err)
		}
		return internet.SpeedTestResult{
			At: at, DownMbps: downMbps, UpMbps: upMbps,
			LatencyMs: idleLatency, LoadedLatencyMs: loadedLatency,
		}
	}
}

func speedTestError(at time.Time, err error) internet.SpeedTestResult {
	msg := err.Error()
	return internet.SpeedTestResult{At: at, Error: &msg}
}

// cloudflareLatency mide el tiempo de ida y vuelta de una petición mínima
// (ver docs/agente-api.md: "latencia en reposo"): GET __down?bytes=0, sin
// cuerpo que descargar.
func cloudflareLatency(ctx context.Context, client *http.Client, baseURL string) (float64, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/__down?bytes=0", nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("speedtest: latencia: status %d", resp.StatusCode)
	}
	return msOfSeconds(time.Since(start)), nil
}

// cloudflareDownloadWithLatency mide la velocidad de bajada (GET
// __down?bytes=cloudflareDownloadBytes, cortada a cloudflareTransferBudget
// si tarda más) y, EN PARALELO mientras baja, la latencia "con carga" (ver
// docs/agente-api.md): varias cloudflareLatency intercaladas, de las que
// se reporta la mediana.
func cloudflareDownloadWithLatency(ctx context.Context, client *http.Client, baseURL string) (downMbps, loadedLatencyMs float64, err error) {
	ctx, cancel := context.WithTimeout(ctx, cloudflareTransferBudget)
	defer cancel()

	done := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var latencies []float64

	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				if ms, lerr := cloudflareLatency(ctx, client, baseURL); lerr == nil {
					mu.Lock()
					latencies = append(latencies, ms)
					mu.Unlock()
				}
			}
		}
	}()

	mbps, derr := cloudflareDownload(ctx, client, baseURL)
	close(done)
	wg.Wait()
	if derr != nil {
		return 0, 0, derr
	}
	mu.Lock()
	defer mu.Unlock()
	return mbps, internet.Median(latencies), nil
}

func cloudflareDownload(ctx context.Context, client *http.Client, baseURL string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/__down?bytes=%d", baseURL, cloudflareDownloadBytes), nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("speedtest: bajada: status %d", resp.StatusCode)
	}

	start := time.Now()
	n, err := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start)
	// Un corte por el presupuesto de tiempo (ctx.Done) con BYTES YA
	// LEÍDOS no es una falla: se calcula Mbps con lo transferido hasta
	// ahí (ver el comentario de cloudflareTransferBudget). Solo sin
	// bytes es un error real (servidor caído, red cortada de verdad).
	if err != nil && n == 0 {
		return 0, err
	}
	if elapsed <= 0 {
		return 0, errors.New("speedtest: bajada instantánea, no se puede medir")
	}
	return mbpsOf(n, elapsed), nil
}

// countingReader cuenta cuántos bytes de verdad se leyeron de r, para que
// cloudflareUpload pueda calcular Mbps con lo transferido hasta un corte
// por tiempo (mismo criterio que cloudflareDownload: ver el comentario de
// cloudflareTransferBudget), no solo con un éxito completo.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func cloudflareUpload(ctx context.Context, client *http.Client, baseURL string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, cloudflareTransferBudget)
	defer cancel()

	body := make([]byte, cloudflareUploadBytes)
	cr := &countingReader{r: bytes.NewReader(body)}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/__up", cr)
	if err != nil {
		return 0, err
	}
	req.ContentLength = int64(len(body))

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		if cr.n == 0 {
			return 0, err
		}
		// Se cortó a mitad (presupuesto de tiempo): igual se puede
		// estimar con lo que de verdad se mandó.
	} else {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("speedtest: subida: status %d", resp.StatusCode)
		}
	}
	if elapsed <= 0 {
		return 0, errors.New("speedtest: subida instantánea, no se puede medir")
	}
	return mbpsOf(cr.n, elapsed), nil
}

func mbpsOf(bytes int64, elapsed time.Duration) float64 {
	return float64(bytes) * 8 / 1_000_000 / elapsed.Seconds()
}

// speedTestRunner programa las pruebas de velocidad (ver
// docs/agente-api.md: "Programadas speedTestsPerDay veces al día,
// repartidas, sin correr durante una caída; también manual (POST, 409 si
// ya hay una)"). Vive mientras dure el proceso: Run bloquea, quien llama
// lo corre en su propia goroutine.
type speedTestRunner struct {
	st         *store.Store
	outageOpen func() bool
	run        speedTestFunc
	log        *slog.Logger

	mu      sync.Mutex
	running bool
	// nextAt: próxima corrida programada. Cero = sin programar (apagado,
	// speedTestsPerDay <= 0).
	nextAt time.Time
}

// newSpeedTestRunner crea el runner con la primera corrida programada
// start+initialDelay (ver Run): un margen para no competir con el
// arranque del agente (primer escaneo, mDNS, etc.) ni, en pruebas, tener
// que esperar un ciclo completo.
func newSpeedTestRunner(st *store.Store, outageOpen func() bool, run speedTestFunc, start time.Time, initialDelay time.Duration, log *slog.Logger) *speedTestRunner {
	return &speedTestRunner{st: st, outageOpen: outageOpen, run: run, log: log, nextAt: start.Add(initialDelay)}
}

// Run revisa cada checkInterval si toca una corrida programada (ver
// maybeRun), hasta que ctx se cancele.
func (r *speedTestRunner) Run(ctx context.Context, checkInterval time.Duration) {
	t := time.NewTicker(checkInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			r.maybeRun(ctx, now)
		}
	}
}

func (r *speedTestRunner) maybeRun(ctx context.Context, now time.Time) {
	r.mu.Lock()
	due := !r.running && !r.nextAt.IsZero() && !now.Before(r.nextAt)
	r.mu.Unlock()
	if !due {
		return
	}
	settings, err := r.st.InternetSettings(ctx)
	if err != nil {
		r.log.Error("speedtest: leer ajustes", "error", err)
		return
	}
	if settings.SpeedTestsPerDay <= 0 {
		r.mu.Lock()
		r.nextAt = time.Time{}
		r.mu.Unlock()
		return
	}
	if r.outageOpen() {
		// Sin correr durante una caída (ver docs/agente-api.md); se
		// reintenta en la próxima vuelta de maybeRun (no se reprograma:
		// nextAt se queda vencida hasta que la caída termine).
		return
	}
	r.TriggerNow(ctx)
}

// TriggerNow intenta arrancar una prueba AHORA (manual o programada):
// false si ya hay una corriendo (ver POST /api/v1/internet/speedtest,
// "409 si ya hay una"). A diferencia de maybeRun, un trigger MANUAL no se
// bloquea por una caída abierta: si la red está caída, la prueba
// simplemente fallará por timeout y quedará guardada con su error (ver
// docs/agente-api.md: "se guarda el error si falla") — correcto sin
// necesitar un caso aparte.
func (r *speedTestRunner) TriggerNow(ctx context.Context) bool {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return false
	}
	r.running = true
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.mu.Unlock()
		}()

		result := r.run(ctx)
		if _, err := r.st.RecordSpeedTest(ctx, result); err != nil {
			r.log.Error("speedtest: guardar resultado", "error", err)
		}
		r.scheduleNext(ctx)
	}()
	return true
}

// scheduleNext reparte la siguiente corrida a partir de AHORA (ver
// docs/agente-api.md: "repartidas"): 24h / speedTestsPerDay desde que
// ÉSTA terminó, sea que haya salido bien o mal (un error no debe dejar de
// intentarlo nunca más).
func (r *speedTestRunner) scheduleNext(ctx context.Context) {
	settings, err := r.st.InternetSettings(ctx)
	if err != nil || settings.SpeedTestsPerDay <= 0 {
		r.mu.Lock()
		r.nextAt = time.Time{}
		r.mu.Unlock()
		return
	}
	interval := 24 * time.Hour / time.Duration(settings.SpeedTestsPerDay)
	r.mu.Lock()
	r.nextAt = time.Now().UTC().Add(interval)
	r.mu.Unlock()
}

// Running: true si hay una prueba en curso (manual o programada).
func (r *speedTestRunner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}
