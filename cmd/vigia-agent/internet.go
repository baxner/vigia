package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/api"
	"github.com/baxner/vigia/agent/internet"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/wake"
	"github.com/baxner/vigia/agent/webhooks"
	"github.com/baxner/vigia/core"
)

// internetMeasureInterval: cada cuánto se mide la calidad del internet
// (ver docs/agente-api.md, "Internet (v1.4)" > Medición: "cada 30 s").
const internetMeasureInterval = 30 * time.Second

// internetProbeTimeout: tope por intento de medición (ver
// docs/agente-api.md: "Timeouts cortos (1.5 s por intento)").
const internetProbeTimeout = 1500 * time.Millisecond

// internetMedianWindow: cuánta historia de internetMs usa EvaluateState
// para "degraded" (ver docs/agente-api.md: "latencia > 3× la mediana de
// 24 h").
const internetMedianWindow = 24 * time.Hour

// ipv6RecheckInterval: cada cuánto se repite la prueba de salida por IPv6
// (ver docs/agente-api.md: "repetirlo cada 10 min basta") — las demás
// vueltas del medidor de 30 s reutilizan el último resultado sin volver a
// tocar la red.
const ipv6RecheckInterval = 10 * time.Minute

// internetDestinations: a quién se mide el tramo "internet" (ver
// docs/agente-api.md: "1.1.1.1 y 8.8.8.8, con conexión TCP al 443").
var internetDestinations = []string{"1.1.1.1", "8.8.8.8"}

// dnsProbeHost: qué nombre se resuelve para medir DNS. Reutiliza
// speed.cloudflare.com (el mismo host de la prueba de velocidad, ver
// speedtest.go) en vez de sumar un tercer dominio a la lista de lo único
// que el agente toca en internet (ver docs/plan.md, Fase 7).
const dnsProbeHost = "speed.cloudflare.com"

// ipv6ProbeAddr: el destino de la prueba de salida por IPv6 (ver
// docs/agente-api.md).
const ipv6ProbeAddr = "[2606:4700:4700::1111]:443"

// msOf: una time.Duration como float64 de milisegundos (con fracción:
// importa para tramos de la LAN, que pueden andar por debajo de 1 ms).
func msOf(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// probeTCPSegment mide un tramo por TCP connect a addr (ver
// docs/agente-api.md: "TCP a un puerto abierto o ICMP sin privilegios").
// Una conexión ACEPTADA o RECHAZADA (RST) cuenta como respuesta —el
// tiempo hasta el RST es una medición de RTT real, mismo criterio que
// hostAlive en core/netutil.go—; solo un timeout sin ninguna respuesta
// (puerto filtrado, nadie contesta) es pérdida de verdad.
func probeTCPSegment(ctx context.Context, addr string, timeout time.Duration) internet.Segment {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start)
	if err == nil {
		conn.Close()
		return internet.Segment{MS: msOf(elapsed), OK: true}
	}
	if errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "refused") {
		return internet.Segment{MS: msOf(elapsed), OK: true}
	}
	return internet.Segment{OK: false}
}

// probeLANSegment mide un tramo de la LAN (router o módem): TCP al puerto
// 80 primero —confirmado en la red real que el Tenda y la ONT FiberHome
// tienen ahí su página de administración, ver docs/plan.md, Fase 5— y,
// solo si eso no contestó NADA (ni accept ni RST: un firewall que
// descarta en silencio el 80 sin bloquear ICMP), un ICMP echo sin
// privilegios de refuerzo (core.PingRTT) antes de darlo por perdido. Dos
// intentos como mucho, cada uno con su propio timeout: nunca más de
// 2×internetProbeTimeout por tramo.
//
// Decisión (ver docs/plan.md, Fase 7: "elige lo confiable y explícalo"):
// TCP primero porque es lo más barato de verificar sin red real (un
// socket más, sin el truco de "udp4" de ICMP) y porque un RST es una
// medición de RTT tan buena como un echo reply; ICMP de refuerzo cubre el
// caso de un filtro que bloquee justo el 80 pero no ICMP.
func probeLANSegment(ctx context.Context, host string, timeout time.Duration) internet.Segment {
	if seg := probeTCPSegment(ctx, net.JoinHostPort(host, "80"), timeout); seg.OK {
		return seg
	}
	if rtt, ok := core.PingRTT(ctx, host, timeout); ok {
		return internet.Segment{MS: msOf(rtt), OK: true}
	}
	return internet.Segment{OK: false}
}

// probeInternetSegment mide el tramo "internet": TCP al 443 de
// internetDestinations EN PARALELO. Basta que uno conteste para que el
// tramo cuente como arriba (ms = el promedio de los que sí contestaron);
// los dos fallando es la única pérdida real de este tramo — es
// agent/internet.OutageDetector, no esta función, quien decide si eso es
// una caída (ver docs/agente-api.md: "2 mediciones seguidas").
func probeInternetSegment(ctx context.Context, timeout time.Duration) internet.Segment {
	results := make([]internet.Segment, len(internetDestinations))
	var wg sync.WaitGroup
	for i, host := range internetDestinations {
		wg.Add(1)
		go func(i int, host string) {
			defer wg.Done()
			results[i] = probeTCPSegment(ctx, net.JoinHostPort(host, "443"), timeout)
		}(i, host)
	}
	wg.Wait()

	var sum float64
	var n int
	for _, r := range results {
		if r.OK {
			sum += r.MS
			n++
		}
	}
	if n == 0 {
		return internet.Segment{OK: false}
	}
	return internet.Segment{MS: sum / float64(n), OK: true}
}

// probeDNSSegment mide cuánto tarda resolver dnsProbeHost con el
// resolvedor del sistema (ver docs/agente-api.md: "el tiempo de resolver
// un nombre conocido con el resolvedor del sistema").
func probeDNSSegment(ctx context.Context, timeout time.Duration) internet.Segment {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	if _, err := net.DefaultResolver.LookupHost(ctx, dnsProbeHost); err != nil {
		return internet.Segment{OK: false}
	}
	return internet.Segment{MS: msOf(time.Since(start)), OK: true}
}

// hasGlobalIPv6 busca una dirección IPv6 global entre las interfaces
// activas (ver docs/agente-api.md: "si el host tiene IPv6 global").
func hasGlobalIPv6() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip.To4() != nil {
			continue // IPv4: no cuenta para esta prueba.
		}
		if ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

// probeIPv6Dial intenta salir por IPv6 hacia ipv6ProbeAddr (ver
// docs/agente-api.md).
func probeIPv6Dial(ctx context.Context, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp6", ipv6ProbeAddr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// measureFunc hace UNA vuelta del medidor de 30 s (ver docs/agente-api.md,
// "Medición"): router/módem/internet/DNS y el estado de IPv6. Inyectable
// (mismo patrón que scheduler.ScanFunc) para poder probar internetMonitor
// sin red real — ver internet_test.go.
type measureFunc func(ctx context.Context) (internet.Sample, internet.IPv6Status)

// networkInfoFunc da la red y el gateway del anillo interior del ÚLTIMO
// escaneo (ver innerNetwork en run.go): el medidor de internet corre en
// su propio ciclo de 30 s, independiente del escaneo de 5 min, y necesita
// saber a quién medir y a qué red atribuir sus alertas.
type networkInfoFunc func() (key, routerGateway string)

// newRealMeasure arma el measureFunc de verdad para el agente en marcha:
// mide router (si se conoce su gateway) y módem (si el anillo exterior ya
// se autodetectó, ver outerRingConfig en router.go) en paralelo con
// internet/DNS, y solo repite IPv6 cada ipv6RecheckInterval.
func newRealMeasure(netInfo networkInfoFunc, outer *outerRingConfig) measureFunc {
	var mu sync.Mutex
	var lastIPv6Check time.Time
	lastIPv6 := internet.IPv6None

	return func(ctx context.Context) (internet.Sample, internet.IPv6Status) {
		_, routerGW := netInfo()
		_, modemGW, modemKnown := outer.current()

		var routerSeg, modemSeg, inetSeg, dnsSeg internet.Segment
		var wg sync.WaitGroup
		if routerGW != "" {
			wg.Add(1)
			go func() { defer wg.Done(); routerSeg = probeLANSegment(ctx, routerGW, internetProbeTimeout) }()
		}
		if modemKnown && modemGW != "" {
			wg.Add(1)
			go func() { defer wg.Done(); modemSeg = probeLANSegment(ctx, modemGW, internetProbeTimeout) }()
		}
		wg.Add(2)
		go func() { defer wg.Done(); inetSeg = probeInternetSegment(ctx, internetProbeTimeout) }()
		go func() { defer wg.Done(); dnsSeg = probeDNSSegment(ctx, internetProbeTimeout) }()
		wg.Wait()

		sample := internet.Sample{At: time.Now().UTC(), Internet: inetSeg, DNS: dnsSeg}
		if routerGW != "" {
			sample.Router = &routerSeg
		}
		if modemKnown && modemGW != "" {
			sample.Modem = &modemSeg
		}

		mu.Lock()
		needsCheck := lastIPv6Check.IsZero() || time.Since(lastIPv6Check) >= ipv6RecheckInterval
		mu.Unlock()

		var status internet.IPv6Status
		if needsCheck {
			hasGlobal := hasGlobalIPv6()
			dialOK := hasGlobal && probeIPv6Dial(ctx, internetProbeTimeout)
			status = internet.ClassifyIPv6(hasGlobal, dialOK)
			mu.Lock()
			lastIPv6, lastIPv6Check = status, time.Now()
			mu.Unlock()
		} else {
			mu.Lock()
			status = lastIPv6
			mu.Unlock()
		}
		return sample, status
	}
}

// internetMonitor corre el ciclo de 30 s de "Internet (v1.4)" (ver
// docs/agente-api.md): alimenta agent/internet.Window (minuto y hora) y
// agent/internet.OutageDetector con cada Sample, persiste lo que
// corresponda y dispara INTERNET_DOWN/INTERNET_RESTORED (vía
// persistAndDispatch, el mismo helper que usan las demás reglas de
// alerts.go). Vive mientras dure el proceso: Run bloquea, quien llama lo
// corre en su propia goroutine (igual que scheduler.Scheduler.Run).
type internetMonitor struct {
	st      *store.Store
	hub     *api.Hub
	sender  *webhooks.Sender
	id      agent.Identity
	log     *slog.Logger
	measure measureFunc
	netInfo networkInfoFunc

	mu                sync.Mutex
	minuteWin         *internet.Window
	hourWin           *internet.Window
	detector          *internet.OutageDetector
	wake              *wake.Detector
	openOutageID      int64
	last              internet.Sample
	haveLast          bool
	ipv6              internet.IPv6Status
	state             internet.State
	stateSince        time.Time
	lastMinuteLossPct float64
}

// newInternetMonitor crea el monitor y resume la caída que hubiera
// quedado abierta de un reinicio a mitad (ver agent/store.Store.OpenOutage
// y agent/internet.ResumeOutageDetector): los CONTADORES del detector se
// pierden igual (ver el comentario de OutageDetector), pero
// GET /api/v1/internet no "olvida" que seguía caído.
func newInternetMonitor(st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, measure measureFunc, netInfo networkInfoFunc, log *slog.Logger) *internetMonitor {
	m := &internetMonitor{
		st: st, hub: hub, sender: sender, id: id, log: log, measure: measure, netInfo: netInfo,
		minuteWin: internet.NewWindow(time.Minute),
		hourWin:   internet.NewWindow(time.Hour),
		detector:  internet.NewOutageDetector(),
		// factor 3 (90 s de umbral sobre un ciclo de 30 s): de sobra por
		// encima del jitter normal de un tick (GC, carga de la máquina),
		// muy por debajo de lo que tarda una suspensión real (ver
		// agent/wake y process, "Suspensión").
		wake:       wake.NewDetector(internetMeasureInterval, 3),
		ipv6:       internet.IPv6None,
		state:      internet.StateUp,
		stateSince: time.Now().UTC(),
	}
	if id, o, err := st.OpenOutage(context.Background()); err == nil {
		m.openOutageID = id
		m.detector = internet.ResumeOutageDetector(o)
		m.state = internet.StateDown
		m.stateSince = o.Start
	} else if !errors.Is(err, store.ErrNotFound) {
		log.Error("internet: leer caída abierta al arrancar", "error", err)
	}
	return m
}

// Run mide cada internetMeasureInterval hasta que ctx se cancele, y
// cierra (Flush) las ventanas abiertas al apagar para no perder el último
// minuto/hora parcial.
func (m *internetMonitor) Run(ctx context.Context) {
	t := time.NewTicker(internetMeasureInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.flush()
			return
		case <-t.C:
			m.tick(ctx)
		}
	}
}

func (m *internetMonitor) tick(ctx context.Context) {
	sample, ipv6 := m.measure(ctx)
	m.process(ctx, sample, ipv6)
}

// process procesa UNA Sample ya medida: separado de tick para que
// internet_test.go pueda alimentar muestras a mano sin pasar por
// measureFunc (ver TestInternetMonitor_*).
func (m *internetMonitor) process(ctx context.Context, sample internet.Sample, ipv6 internet.IPv6Status) {
	m.mu.Lock()
	m.last, m.haveLast = sample, true
	m.ipv6 = ipv6
	// Suspensión (ver docs/agente-api.md, "Internet (v1.4)" > Suspensión):
	// un hueco mucho más grande que el ciclo de 30 s entre esta muestra y
	// la anterior probablemente sea la máquina despertando, no una caída
	// real — Reset() evita que una falla de ANTES del hueco se combine
	// con esta como si fueran la misma racha (ver el comentario de
	// OutageDetector.Reset). Window.Feed no necesita este cuidado: un
	// hueco grande solo le hace cerrar la ventana vieja y abrir la nueva
	// con esta muestra, sin inventar minutos/horas de por medio.
	if asleep, gap := m.wake.Feed(sample.At); asleep {
		m.log.Warn("internet: hueco grande entre muestras, posible suspensión de la máquina; se reinician las rachas de caída", "hueco", gap)
		m.detector.Reset()
	}
	minuteAgg, minuteClosed := m.minuteWin.Feed(sample)
	hourAgg, hourClosed := m.hourWin.Feed(sample)
	if minuteClosed {
		m.lastMinuteLossPct = minuteAgg.Internet.LossPct()
	}
	outageEvent := m.detector.Feed(sample)
	openNow := m.detector.Open()
	m.mu.Unlock()

	if minuteClosed {
		if err := m.st.RecordMinuteAgg(ctx, minuteAgg); err != nil {
			m.log.Error("internet: guardar minuto", "error", err)
		}
	}
	if hourClosed {
		if err := m.st.RecordHourAgg(ctx, hourAgg); err != nil {
			m.log.Error("internet: guardar hora", "error", err)
		}
	}
	if outageEvent != nil {
		m.handleOutageEvent(ctx, *outageEvent)
	}
	m.updateState(ctx, sample, openNow, outageEvent)
}

func (m *internetMonitor) handleOutageEvent(ctx context.Context, o internet.Outage) {
	networkKey, _ := m.netInfo()
	settings, err := m.st.AlertSettings(ctx)
	if err != nil {
		m.log.Error("alert_settings: leer", "error", err)
		settings = alerts.DefaultSettings()
	}

	if o.End == nil {
		id, err := m.st.InsertOutageOpen(ctx, o)
		if err != nil {
			m.log.Error("internet: guardar caída", "error", err)
		} else {
			m.mu.Lock()
			m.openOutageID = id
			m.mu.Unlock()
		}
		if a := alerts.EvaluateInternetDown(o.Blame, networkKey, o.Start, settings); a != nil {
			persistAndDispatch(ctx, m.st, m.hub, m.sender, m.id, *a, nil, m.log)
		}
		return
	}

	m.mu.Lock()
	id := m.openOutageID
	m.openOutageID = 0
	m.mu.Unlock()
	if id != 0 {
		if err := m.st.CloseOutage(ctx, id, *o.End); err != nil {
			m.log.Error("internet: cerrar caída", "error", err)
		}
	}
	if a := alerts.EvaluateInternetRestored(o.Blame, networkKey, o.Start, *o.End, settings); a != nil {
		persistAndDispatch(ctx, m.st, m.hub, m.sender, m.id, *a, nil, m.log)
	}
}

// updateState evalúa el estado agregado y, si cambió, actualiza
// state/stateSince. outageEvent es la Outage que el detector acaba de
// abrir/cerrar en ESTE tick (nil si ninguna de las dos pasó, ver
// agent/internet.OutageDetector.Feed): cuando no es nil, el "desde cuándo"
// del nuevo estado debe ser el instante REAL de la transición (o.Start al
// abrir, *o.End al cerrar) y no sample.At (la muestra que recién lo
// CONFIRMÓ, dos mediciones después) — mismo principio que separa Start/End
// de la muestra que los confirma en OutageDetector.
func (m *internetMonitor) updateState(ctx context.Context, sample internet.Sample, outageOpen bool, outageEvent *internet.Outage) {
	m.mu.Lock()
	lossPct := m.lastMinuteLossPct
	m.mu.Unlock()

	median := m.median24h(ctx)
	newState := internet.EvaluateState(outageOpen, lossPct, sample.Internet.MS, median)

	since := sample.At
	if outageEvent != nil {
		if outageEvent.End != nil {
			since = *outageEvent.End
		} else {
			since = outageEvent.Start
		}
	}

	m.mu.Lock()
	if newState != m.state {
		m.state = newState
		m.stateSince = since
	}
	m.mu.Unlock()
}

func (m *internetMonitor) median24h(ctx context.Context) float64 {
	rows, err := m.st.RecentMinutes(ctx, internetMedianWindow)
	if err != nil {
		m.log.Error("internet: leer historia para mediana", "error", err)
		return 0
	}
	vals := make([]float64, 0, len(rows))
	for _, r := range rows {
		if r.Internet.HasLatency() {
			vals = append(vals, r.Internet.AvgMs)
		}
	}
	return internet.Median(vals)
}

// flush cierra (si tenían algo) las ventanas de minuto/hora abiertas, al
// apagar el agente — con un contexto propio y corto: el de Run ya está
// cancelado para cuando esto corre.
func (m *internetMonitor) flush() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.mu.Lock()
	minuteAgg, minuteClosed := m.minuteWin.Flush()
	hourAgg, hourClosed := m.hourWin.Flush()
	m.mu.Unlock()
	if minuteClosed {
		if err := m.st.RecordMinuteAgg(ctx, minuteAgg); err != nil {
			m.log.Error("internet: guardar último minuto al apagar", "error", err)
		}
	}
	if hourClosed {
		if err := m.st.RecordHourAgg(ctx, hourAgg); err != nil {
			m.log.Error("internet: guardar última hora al apagar", "error", err)
		}
	}
}

// InternetNow da el estado en vivo para GET /api/v1/internet (ver
// agent/api.internetProvider): el último estado evaluado, desde cuándo, la
// última Sample medida y el estado de IPv6.
func (m *internetMonitor) InternetNow() (state internet.State, since time.Time, now internet.Sample, ipv6 internet.IPv6Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, m.stateSince, m.last, m.ipv6
}

// OutageOpen: true si hay una caída de internet abierta ahora (ver
// speedtest.go: "sin correr durante una caída").
func (m *internetMonitor) OutageOpen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.detector.Open()
}

// internetAPIAdapter junta internetMonitor (estado en vivo) y
// speedTestRunner (arrancar pruebas) detrás de la interfaz que pide
// agent/api (ver api.Deps.Internet, internetProvider): dos componentes
// separados en este paquete, una sola interfaz del lado de la API. ctx es
// el del ciclo de vida del agente (el mismo que recibe el Scheduler):
// TriggerSpeedTest arranca una goroutine que sigue corriendo después de
// que el 202 ya se mandó, así que no puede depender del contexto de ESE
// request HTTP (ver speedTestRunner.TriggerNow).
type internetAPIAdapter struct {
	monitor *internetMonitor
	speed   *speedTestRunner
	ctx     context.Context
}

func (a internetAPIAdapter) InternetNow() (internet.State, time.Time, internet.Sample, internet.IPv6Status) {
	return a.monitor.InternetNow()
}

func (a internetAPIAdapter) TriggerSpeedTest() bool {
	return a.speed.TriggerNow(a.ctx)
}
