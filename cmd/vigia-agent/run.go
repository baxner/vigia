package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/api"
	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/mdns"
	"github.com/baxner/vigia/agent/rotatelog"
	"github.com/baxner/vigia/agent/scheduler"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/wake"
	"github.com/baxner/vigia/agent/webhooks"
	"github.com/baxner/vigia/core"
)

// innerNetwork: red y gateway del anillo interior del ÚLTIMO escaneo (ver
// recordScan más abajo), para que internetMonitor —que corre en su
// propio ciclo de 30 s, independiente del escaneo de 5 min— sepa a quién
// medir como "router" y a qué red atribuir sus alertas. Mismo patrón que
// scanHealth: atomic.Value en vez de un mutex aparte.
var innerNetwork atomic.Value // guarda innerNetworkInfo

type innerNetworkInfo struct {
	key     string
	gateway string
}

// innerNetworkKeyAndGateway: lee innerNetwork como networkInfoFunc (ver
// internet.go). "", "" si todavía no corrió ningún escaneo interior.
func innerNetworkKeyAndGateway() (key, gateway string) {
	v, _ := innerNetwork.Load().(innerNetworkInfo)
	return v.key, v.gateway
}

// outerRingInterval: cada cuánto se escanea el anillo exterior (ver
// docs/plan.md, Fase 5: "cada 10 min"). A diferencia del anillo
// interior, fijo: no hay flag para cambiarlo (el exterior es
// best-effort — sin MAC ni multicast — y no necesita la misma frecuencia).
const outerRingInterval = 10 * time.Minute

const defaultPort = 7341

// version: lo que devuelve GET /api/v1/info. Sin infraestructura de build
// con -ldflags todavía (el agente es nuevo); se sube a mano por release.
const version = "0.1.0"

// runCmd es lo que ejecuta el LaunchAgent (ver install_darwin.go): primer
// plano, un solo proceso, sale limpio con SIGINT/SIGTERM.
func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	interval := fs.Duration("interval", scheduler.DefaultInterval, "intervalo entre escaneos")
	port := fs.Int("port", defaultPort, "puerto HTTPS")
	bind := fs.String("bind", fmt.Sprintf(":%d", defaultPort), "dirección de escucha HTTP/HTTPS (p. ej. :7341 o 127.0.0.1:7341)")
	dataDir := fs.String("data", "", "directorio de datos/logs (vacío = el del sistema, ver docs/agente-api.md)")
	name := fs.String("name", "", "nombre del agente (vacío = el del equipo)")
	noMDNS := fs.Bool("no-mdns", false, "no anunciarse por mDNS/Bonjour")
	// TLS v1.5 (ver docs/agente-api.md): HTTPS en -port con el certificado
	// propio del agente y, en el MISMO puerto, HTTP sin cifrar mientras dure
	// la transición (ver agent.NewDualListener): una app de antes de esta
	// fase sigue funcionando sin cambios hasta que se actualiza y fija la
	// huella. Encendido por defecto EN ESTA VERSIÓN: se apaga con
	// -http-legacy=false cuando ya no quede ningún teléfono por migrar.
	httpLegacy := fs.Bool("http-legacy", true, "aceptar también HTTP sin cifrar en el mismo puerto (transición TLS v1.5)")
	// Anillo exterior (Fase 5, ver docs/agente-api.md, "Seguridad v1.2" >
	// Anillos): "" autodetecta desde el router (UPnP/NAT-PMP), "off" lo
	// apaga, cualquier otra cosa es un CIDR fijo.
	outerFlag := fs.String("outer", "", "anillo exterior: CIDR fijo, \"off\" para apagarlo, o vacío para autodetectar")
	outerGatewayFlag := fs.String("outer-gateway", "", "puerta de enlace del anillo exterior (vacío = el \".1\" del CIDR de -outer)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var bindSet, portSet bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "bind":
			bindSet = true
		case "port":
			portSet = true
		}
	})
	listenAddr := *bind
	listenPort := *port
	if !bindSet && portSet {
		listenAddr = fmt.Sprintf(":%d", *port)
	} else if bindSet {
		if _, pStr, err := net.SplitHostPort(listenAddr); err == nil {
			if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
				listenPort = p
			}
		} else {
			listenAddr = net.JoinHostPort(listenAddr, strconv.Itoa(*port))
		}
	}
	outer, err := newOuterRingConfig(*outerFlag, *outerGatewayFlag)
	if err != nil {
		return err
	}

	paths, err := agent.DefaultPaths(*dataDir)
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	// agent.log rota por tamaño dentro del propio proceso (ver
	// agent/rotatelog y docs/agente-api.md, "Rotación de logs"): sin
	// esto, el archivo que escribe el LaunchAgent (antes, por la
	// redirección cruda de stderr de launchd) crecía sin límite durante
	// toda la vida del servicio. Se copia a stderr SOLO si es una
	// terminal (`vigia-agent run` a mano): bajo el LaunchAgent, stderr va
	// a agent.log.launchd (ver install_darwin.go), que no se rota y debe
	// quedarse para lo que el runtime de Go escriba por su cuenta (un
	// panic, o antes de esta línea).
	logWriter, err := rotatelog.Open(paths.LogPath(), rotatelog.DefaultMaxBytes, rotatelog.DefaultKeep)
	if err != nil {
		return fmt.Errorf("abrir log: %w", err)
	}
	defer logWriter.Close()
	var logOut io.Writer = logWriter
	if stderrIsTerminal() {
		logOut = io.MultiWriter(os.Stderr, logWriter)
	}
	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("vigia-agent: arrancando", "data", paths.DataDir, "log", paths.LogDir)

	id, err := agent.LoadOrCreateIdentity(paths.DataDir, *name)
	if err != nil {
		return fmt.Errorf("identidad: %w", err)
	}
	log.Info("identidad", "id", id.ID, "name", id.Name)

	// TLS v1.5 (ver docs/agente-api.md): certificado propio, generado una
	// sola vez y reutilizado entre reinicios (ver agent.LoadOrCreateCert).
	cert, err := agent.LoadOrCreateCert(paths.DataDir)
	if err != nil {
		return fmt.Errorf("certificado TLS: %w", err)
	}
	log.Info("certificado TLS", "huella", cert.FingerprintSHA256)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	st, err := store.Open(ctx, paths.DBPath())
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer st.Close()
	var loops sync.WaitGroup
	defer func() {
		cancel()
		loops.Wait()
	}()

	hub := api.NewHub()
	// Un solo Sender para toda la vida del proceso: lo usan tanto el envío
	// de verdad (recordScan → dispatchWebhooks, ver alerts.go) como
	// POST /api/v1/webhooks/{id}/test desde la API — mismo User-Agent,
	// mismo *http.Client.
	sender := webhooks.NewSender(version)

	// scanGate serializa el anillo interior y el exterior: "uno a la vez"
	// (ver docs/plan.md, Fase 5) sin que ninguno de los dos
	// tenga que saber del otro — ambos pasan por acquireScan antes de
	// tocar la red. Token disponible desde el arranque (buffer 1, lleno).
	scanGate := make(chan struct{}, 1)
	scanGate <- struct{}{}
	acquireScan := func(ctx context.Context) (release func(), err error) {
		select {
		case <-scanGate:
			return func() { scanGate <- struct{}{} }, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	scan := func(ctx context.Context) (*core.ScanResult, error) {
		release, err := acquireScan(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		// Sin SkipNeighborTable: en macOS y Linux sí hay tabla ARP/NDP (a
		// diferencia de Android/iOS, que la bloquean) — ahí salen las MAC.
		return core.Scan(ctx, core.ScanOptions{})
	}
	// scanWake detecta, entre un escaneo registrado y el siguiente, un
	// hueco sospechoso de suspensión de la máquina (ver agent/wake y
	// docs/agente-api.md, "Suspensión"): factor 2 sobre *interval, igual
	// razonamiento que scheduler.wakeFactor — cómodamente por debajo del
	// umbral de 15 min de IMPORTANT_OFFLINE, así que cualquier hueco
	// capaz de fabricar esa alerta sin que la máquina estuviera dormida
	// de verdad ya se detecta aquí. Un solo Detector para toda la vida
	// del proceso, alimentado SIEMPRE desde este closure (el gate
	// scanGate/inFlight del scheduler garantiza que nunca corre dos
	// escaneos a la vez, así que esto nunca se alimenta en paralelo).
	scanWake := wake.NewDetector(*interval, 2.0)
	record := func(ctx context.Context, res *core.ScanResult, at time.Time) {
		justWoke, gap := scanWake.Feed(at)
		if justWoke {
			log.Warn("escaneo: hueco grande desde el anterior, posible suspensión de la máquina; no se evalúa IMPORTANT_OFFLINE este ciclo", "hueco", gap)
		}
		recordScan(ctx, st, hub, sender, id, outer, res, at, justWoke, log)
	}
	sched := scheduler.New(ctx, scan, record, *interval, log)

	// Anillo exterior (ver docs/agente-api.md, "Seguridad v1.2" > Anillos):
	// SIN marcar al agente como ciego si no encuentra nada — eso lo decide
	// recordOuterScan, no esta fábrica. outerScan devuelve (nil, nil) —no
	// un error— cuando no hay anillo que escanear todavía (apagado, o
	// autodetección sin resultado aún): así no ensucia el log con un
	// "error" cada 10 minutos por algo que no lo es.
	//
	// A diferencia del interior, NO corre scheduler.Scheduler (que
	// escanearía también "al arrancar"): verificado contra la red real,
	// ese primer intento casi siempre pierde la carrera contra el PRIMER
	// ciclo del interior —el que recién AUTODETECTA el anillo exterior vía
	// pollRouter→outer.observeRouter— porque un core.Scan tarda varios
	// segundos y este arrancaría casi en el instante cero; perder esa
	// carrera dejaría al anillo exterior sin un segundo intento hasta los
	// 10 minutos completos. runOuterRingLoop (router.go) usa un ticker liso
	// que solo dispara cada outerRingInterval: para ese entonces el interior
	// ya tuvo de sobra su primer ciclo (siempre inmediato) y, con router
	// que conteste, el anillo exterior ya está autodetectado.
	outerScan := func(ctx context.Context) (*core.ScanResult, error) {
		cidr, gw, ok := outer.current()
		if !ok {
			return nil, nil
		}
		release, err := acquireScan(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		return core.Scan(ctx, core.ScanOptions{CIDR: cidr, Gateway: gw, Routed: true})
	}
	outerRecord := func(ctx context.Context, res *core.ScanResult, at time.Time) {
		if res == nil {
			return
		}
		recordOuterScan(ctx, st, hub, sender, id, res, at, log)
	}

	// Internet (v1.4, Fase 7, ver docs/agente-api.md): su propio ciclo de
	// 30 s, independiente del escaneo de 5 min — measure/HTTP client
	// reales (ver internet.go/speedtest.go); las pruebas usan sus propios
	// fakes, nunca esto.
	internetMeasure := newRealMeasure(innerNetworkKeyAndGateway, outer)
	internetMon := newInternetMonitor(st, hub, sender, id, internetMeasure, innerNetworkKeyAndGateway, log)
	speedClient := &http.Client{}
	speedRunner := newSpeedTestRunner(st, internetMon.OutageOpen,
		newCloudflareSpeedTest(speedClient, cloudflareSpeedTestBaseURL), time.Now().UTC(), 5*time.Minute, log)
	internetAPI := internetAPIAdapter{monitor: internetMon, speed: speedRunner, ctx: ctx}

	srv := api.NewServer(api.Deps{Store: st, Scheduler: sched, Identity: id, Version: version, Hub: hub, Sender: sender,
		Warning: func() string { w, _ := scanHealth.Load().(string); return w }, Internet: internetAPI,
		CertSHA256: cert.FingerprintSHA256})

	// HTTPS (ver TLS v1.5): el certificado propio recién cargado, nunca el
	// de una CA — la app lo fija por huella, no valida la cadena. El
	// listener (ver abajo) es el que hace el handshake, no http.Server.
	// Sin WriteTimeout a propósito: el stream SSE es una respuesta larga. Los
	// demás límites evitan que un cliente de la LAN deje conexiones colgadas.
	httpSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
	}
	ln, err := net.Listen("tcp", httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("api: escuchar %s: %w", httpSrv.Addr, err)
	}
	dualLn := agent.NewDualListener(ln, &tls.Config{
		Certificates: []tls.Certificate{cert.TLS},
		MinVersion:   tls.VersionTLS12,
	}, *httpLegacy)

	var announcer *mdns.Announcer
	if !*noMDNS {
		announcer = mdns.Start(ctx, listenPort, id.ID, id.Name, cert.FingerprintSHA256, log)
		defer announcer.Stop()
	}

	loops.Add(4)
	go func() {
		defer loops.Done()
		sched.Run()
	}()
	go func() {
		defer loops.Done()
		runOuterRingLoop(ctx, outerRingInterval, outerScan, outerRecord, log)
	}()
	go func() {
		defer loops.Done()
		internetMon.Run(ctx)
	}()
	go func() {
		defer loops.Done()
		speedRunner.Run(ctx, time.Minute)
	}()

	serveErr := make(chan error, 1)
	go func() {
		log.Info("api: escuchando", "addr", httpSrv.Addr, "tls", true, "httpLegacy", *httpLegacy, "huellaCert", cert.FingerprintSHA256)
		err := httpSrv.Serve(dualLn)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Warn("api: shutdown", "error", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("vigia-agent: señal recibida, apagando")
	case err := <-serveErr:
		if err != nil {
			log.Error("api: falló", "error", err)
		}
		cancel() // corta también al scheduler y al anuncio mDNS
	}

	log.Info("vigia-agent: terminado")
	return nil
}

// recordScan reconcilia un ScanResult del anillo INTERIOR contra el store,
// publica lo que corresponda en el hub SSE, evalúa las reglas de Alertas
// (v1.1 y las de Fase 5 que miran al interior, ver alerts.go en este
// paquete) y consulta al router (ver router.go): la bisagra entre
// scheduler, store, api, agent/alerts y core — ninguno de esos paquetes
// conoce a los demás. outer es el anillo exterior (ver router.go): este
// ciclo solo lo ALIMENTA (pollRouter → outer.observeRouter), nunca lo
// escanea — eso lo hace el outerSched aparte, con su propio ScanResult.
func recordScan(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, outer *outerRingConfig, res *core.ScanResult, at time.Time, justWoke bool, log *slog.Logger) {
	netKey := string(history.NewNetworkKey(res.Gateway, res.Subnet))
	// Para internetMonitor (ver innerNetworkKeyAndGateway): se guarda
	// ANTES de decidir isBlind —un escaneo ciego igual conoce el gateway
	// del router, que es lo único que internetMonitor necesita de aquí—.
	innerNetwork.Store(innerNetworkInfo{key: netKey, gateway: res.Gateway})

	// Un escaneo sin NINGUNA respuesta activa (solo lo que quedaba en la tabla
	// ARP) no describe la red: algo le impide al agente hablarle — en macOS 15,
	// casi seguro falta el permiso de "Red local" (las conexiones salen
	// bloqueadas por NECP). Registrarlo marcaría desconexiones falsas. Uno sin
	// ningún equipo (ni siquiera este) tampoco: con las alertas, daría un
	// "Equipo importante desconectado" por cada equipo marcado como importante.
	isBlind := activeResponders(res.Devices, res.Gateway) == 0
	warning := ""
	if isBlind {
		warning = noLocalNetworkWarning
	}
	scanHealth.Store(warning)

	// La transición ciego↔normal (AGENT_BLIND/AGENT_RECOVERED) se evalúa
	// SIEMPRE, sea o no este escaneo el que se registra como tal — un
	// escaneo ciego no llega a RecordScan (se corta abajo), pero necesita
	// igual la oportunidad de disparar AGENT_BLIND; uno normal que viene
	// justo después de uno ciego es el que dispara AGENT_RECOVERED. Es,
	// a propósito, la ÚNICA evaluación de salud de todo el agente: el
	// anillo exterior (recordOuterScan) nunca la llama (ver
	// docs/plan.md, Fase 5: "la salud se evalúa SOLO con el
	// anillo interior").
	handleHealthTransition(ctx, st, hub, sender, id, netKey, isBlind, warning, at, log)

	if isBlind {
		log.Warn("escaneo sin respuestas activas: no se registra", "equipos", len(res.Devices), "aviso", noLocalNetworkWarning)
		return
	}

	// ROUTER_CHANGED compara la MAC del gateway ANTES y DESPUÉS de este
	// escaneo (ver agent/alerts.EvaluateRouterChanged): hay que leerla
	// antes de que RecordScan la sobrescriba más abajo.
	prevGatewayMAC := previousGatewayMAC(ctx, st, netKey, log)

	net := store.NetworkInfo{
		Key:     history.NetworkKey(netKey),
		Subnet:  res.Subnet,
		Gateway: res.Gateway,
		Role:    "inner",
	}
	for _, d := range res.Devices {
		if d.IP == res.Gateway && d.MAC != "" {
			net.GatewayMAC = d.MAC
			break
		}
	}

	events, err := st.RecordScan(ctx, net, res.Devices, at)
	if err != nil {
		log.Error("recordScan", "error", err)
		return
	}
	for _, ev := range events {
		hub.PublishDevice(ev)
	}
	hub.PublishScan(netKey, at, time.Now().UTC(), len(res.Devices))
	log.Info("escaneo", "red", net.Key, "dispositivos", len(res.Devices), "eventos", len(events))

	processInnerAlerts(ctx, st, hub, sender, id, netKey, events, res.Devices, res.Gateway, prevGatewayMAC, net.GatewayMAC, at, justWoke, log)
	pollRouter(ctx, st, hub, sender, id, outer, netKey, res.Gateway, at, log)
}

// previousGatewayMAC lee la MAC del gateway del anillo interior GUARDADA
// antes de este escaneo (ver agent/alerts.EvaluateRouterChanged). ""
// tanto si la red es nueva (ErrNotFound: nada que comparar todavía, ni
// falta) como si hubo un error de verdad leyéndola — en ambos casos, más
// vale no arriesgar una falsa alarma que frenar el escaneo por esto.
func previousGatewayMAC(ctx context.Context, st *store.Store, netKey string, log *slog.Logger) string {
	prev, err := st.NetworkByKey(ctx, netKey)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Error("router_changed: leer red anterior", "error", err)
		}
		return ""
	}
	if prev.GatewayMAC == nil {
		return ""
	}
	return *prev.GatewayMAC
}

// recordOuterScan reconcilia un ScanResult del anillo EXTERIOR (ver
// docs/agente-api.md, "Seguridad v1.2" > Anillos): se registra como una
// red más, con role "outer", y solo evalúa NEW_DEVICE/IMPORTANT_*
// (processOuterAlerts) — nunca la salud del agente (ver el comentario en
// recordScan) ni las alertas que son del anillo interior. Sin
// SkipNeighborTable/isBlind: un escaneo Routed nunca lee ARP (ver
// ScanOptions.Routed), así que "ciego" no es un concepto aplicable aquí —
// cero equipos en el anillo exterior es, con frecuencia, la realidad
// (solo el módem).
func recordOuterScan(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, res *core.ScanResult, at time.Time, log *slog.Logger) {
	netKey := string(history.NewNetworkKey(res.Gateway, res.Subnet))
	net := store.NetworkInfo{
		Key:     history.NetworkKey(netKey),
		Subnet:  res.Subnet,
		Gateway: res.Gateway,
		Role:    "outer",
		// GatewayMAC queda "": un escaneo Routed no lee ARP (ver core.Scan),
		// así que nunca hay una MAC de verdad que guardar aquí.
	}

	events, err := st.RecordScan(ctx, net, res.Devices, at)
	if err != nil {
		log.Error("recordOuterScan", "error", err)
		return
	}
	for _, ev := range events {
		hub.PublishDevice(ev)
	}
	hub.PublishScan(netKey, at, time.Now().UTC(), len(res.Devices))
	log.Info("escaneo exterior", "red", net.Key, "dispositivos", len(res.Devices), "eventos", len(events))

	processOuterAlerts(ctx, st, hub, sender, id, netKey, events, log)
}

// noLocalNetworkWarning: lo que ve la app cuando el agente no puede hablar con la LAN.
var noLocalNetworkWarning = localNetworkWarningForOS(runtime.GOOS)

func localNetworkWarningForOS(goos string) string {
	if goos == "darwin" {
		return "El agente no recibe respuesta de la red local. En macOS: Ajustes del Sistema → Privacidad y seguridad → Red local → activa Vigía Agent."
	}
	return "El agente no recibe respuesta de la red local. Verifica permisos de red/firewall o que el contenedor use network_mode: host en Linux/Docker."
}

// scanHealth: aviso del último escaneo ("" = todo bien), para /api/v1/info.
var scanHealth atomic.Value

// activeResponders cuenta los equipos que respondieron algo en este escaneo
// (no solo aparecían en la tabla ARP), sin contar al propio equipo ni al router.
func activeResponders(devs []core.Device, gateway string) int {
	n := 0
	for _, d := range devs {
		// El propio equipo y el router responden aun sin el permiso de Red
		// local (el tráfico al router es la salida a internet): no prueban que
		// el agente vea la LAN.
		if d.Extra["self"] == "true" || d.IP == gateway {
			continue
		}
		for _, s := range d.Sources {
			if s != "arp" && s != "self" && s != "ndp" {
				n++
				break
			}
		}
	}
	return n
}

// stderrIsTerminal: si stderr es una terminal (y no un archivo, como bajo
// el LaunchAgent). Ver el logger de runCmd.
func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
