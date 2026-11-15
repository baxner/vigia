package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/api"
	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/scheduler"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/webhooks"
	"github.com/baxner/vigia/core"
)

// outerRingConfig decide qué anillo exterior escanear (ver
// docs/agente-api.md, "Seguridad v1.2" > Anillos): fijo por flags
// (-outer <cidr> [-outer-gateway <ip>]), apagado (-outer off), o detectado
// solo a partir de RouterInfo.ExternalIP cuando Reach es "outer_ring" —
// nunca con IP pública ("internet"): ahí no hay una red intermedia que
// escanear, es el propio ISP. Seguro para llamar desde dos goroutines: el
// ciclo del anillo interior la actualiza (observeRouter) y el del exterior
// la lee (current) en cada vuelta de su propio temporizador.
type outerRingConfig struct {
	mu    sync.Mutex
	off   bool
	fixed bool

	cidr    string
	gateway string
}

// newOuterRingConfig interpreta -outer/-outer-gateway (ver runCmd en
// run.go). flagValue == "" (el valor por defecto) deja la autodetección
// activada; "off" la apaga por completo; cualquier otra cosa se parsea
// como CIDR fijo.
func newOuterRingConfig(flagValue, gatewayFlag string) (*outerRingConfig, error) {
	c := &outerRingConfig{}
	switch flagValue {
	case "":
		// autodetección (ver observeRouter): nada que fijar todavía.
	case "off":
		c.off = true
	default:
		_, ipNet, err := net.ParseCIDR(flagValue)
		if err != nil {
			return nil, fmt.Errorf("-outer %q: %w", flagValue, err)
		}
		gw := gatewayFlag
		if gw == "" {
			gw = firstHostOf(ipNet)
			if gw == "" {
				return nil, fmt.Errorf("-outer %q: no es una red IPv4, hace falta -outer-gateway", flagValue)
			}
		}
		c.fixed = true
		c.cidr, c.gateway = flagValue, gw
	}
	return c, nil
}

// firstHostOf: el ".1" de ipNet (ver RouterInfo/OuterRing en
// docs/agente-api.md: la puerta de enlace del anillo exterior es ".1" de
// su /24 salvo que -outer-gateway diga otra cosa). "" si ipNet no es IPv4.
func firstHostOf(ipNet *net.IPNet) string {
	v4 := ipNet.IP.To4()
	if v4 == nil {
		return ""
	}
	ip := append(net.IP{}, v4...)
	ip[3] = 1
	return ip.String()
}

// current devuelve el CIDR/gateway a escanear ahora, u ok=false si el
// anillo exterior está apagado o todavía no se autodetectó ninguno (p.
// ej. el router nunca contestó UPnP/NAT-PMP, o contestó con IP pública).
func (c *outerRingConfig) current() (cidr, gateway string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.off {
		return "", "", false
	}
	if c.fixed {
		return c.cidr, c.gateway, true
	}
	if c.cidr == "" {
		return "", "", false
	}
	return c.cidr, c.gateway, true
}

// observeRouter autodetecta el anillo exterior a partir de un RouterInfo
// recién consultado (ver docs/agente-api.md: "si es privada o CGNAT... el
// anillo exterior es su /24 con puerta de enlace en .1"). No hace nada si
// el anillo está fijo o apagado por flags, si info es nil (el router no
// contestó) o si Reach no es "outer_ring" (con IP pública no hay anillo
// intermedio; "unknown" tampoco alcanza para fijar nada). Mismo cálculo
// que ringHint en core/security.go.
func (c *outerRingConfig) observeRouter(info *core.RouterInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.off || c.fixed || info == nil || info.Reach != "outer_ring" {
		return
	}
	ip := net.ParseIP(info.ExternalIP)
	if ip == nil {
		return
	}
	v4 := ip.To4()
	if v4 == nil {
		return
	}
	c.cidr = fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2])
	c.gateway = fmt.Sprintf("%d.%d.%d.1", v4[0], v4[1], v4[2])
}

// pollRouter consulta el router del anillo interior (UPnP + NAT-PMP, ver
// core.QueryRouter), persiste lo que encuentra y evalúa PORT_MAPPING_ADDED
// (ROUTER_CHANGED se evalúa en recordScan/processInnerAlerts, con la MAC
// del propio escaneo ARP — no depende de que el router conteste UPnP).
// Corre en el MISMO ciclo que el escaneo del anillo interior ("cada 5 min
// y al arrancar", ver docs/plan.md, Fase 5): no necesita su
// propio temporizador, así que esta función es una llamada más dentro de
// recordScan, no un Scheduler aparte.
func pollRouter(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, outer *outerRingConfig, netKey, gateway string, at time.Time, log *slog.Logger) {
	info, err := core.QueryRouter(ctx, gateway)
	if err != nil {
		log.Warn("router: consulta", "error", err)
		return
	}
	outer.observeRouter(info)
	if info == nil {
		return // ni UPnP ni NAT-PMP contestaron: se conserva lo último guardado (ver RouterState)
	}
	recordRouterInfo(ctx, st, hub, sender, id, netKey, info, at, log)
}

// recordRouterInfo hace todo lo que pollRouter necesita DESPUÉS de tener
// un *core.RouterInfo ya resuelto (info != nil: pollRouter solo llega
// aquí si contestó UPnP o NAT-PMP) — separado de pollRouter para poder
// probarlo sin red, con un RouterInfo armado a mano (ver router_test.go):
// llena PortMapping.DeviceID con el equipo conocido de esa IP, guarda el
// estado y las redirecciones (ver agent/store.SaveRouterState/
// ReconcilePortMappings) y evalúa PORT_MAPPING_ADDED con línea base (ver
// agent/alerts.EvaluateNewPortMappings).
func recordRouterInfo(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, netKey string, info *core.RouterInfo, at time.Time, log *slog.Logger) {
	known, err := st.DevicesByNetwork(ctx, history.NetworkKey(netKey))
	if err != nil {
		log.Error("router: leer equipos para deviceId", "error", err)
		known = nil
	}
	deviceIDByIP := make(map[string]int64, len(known))
	for _, d := range known {
		deviceIDByIP[d.LastIP] = d.ID
	}
	for i := range info.PortMappings {
		if devID, ok := deviceIDByIP[info.PortMappings[i].InternalIP]; ok {
			info.PortMappings[i].DeviceID = &devID
		}
	}

	settings, err := st.AlertSettings(ctx)
	if err != nil {
		log.Error("alert_settings: leer", "error", err)
		settings = alerts.DefaultSettings()
	}
	previouslyActive, err := st.ActivePortMappings(ctx, netKey)
	if err != nil {
		log.Error("router: leer redirecciones activas", "error", err)
		return
	}
	if !info.MappingsComplete {
		// La lista llegó a medias (o el router no la da por UPnP): no se sabe
		// qué se cerró. Se conservan las de antes, para el informe y para no
		// "redescubrirlas" (y avisar otra vez) en la próxima consulta.
		info.PortMappings = previouslyActive
		if err := st.SaveRouterState(ctx, netKey, info, at); err != nil {
			log.Error("router: guardar estado", "error", err)
		}
		log.Info("router: lista de redirecciones incompleta; se conserva la anterior", "activas", len(previouslyActive))
		return
	}
	seenKeys, err := st.PortMappingNotifyKeysSeen(ctx, netKey)
	if err != nil {
		log.Error("router: leer redirecciones vistas", "error", err)
		return
	}
	hasBaseline, err := st.PortMappingsBaselineDone(ctx)
	if err != nil {
		log.Error("router: leer línea base de redirecciones", "error", err)
		return
	}

	newAlerts := alerts.EvaluateNewPortMappings(seenKeys, info.PortMappings, hasBaseline, netKey, at, settings)

	if err := st.SaveRouterState(ctx, netKey, info, at); err != nil {
		log.Error("router: guardar estado", "error", err)
	}
	if err := st.ReconcilePortMappings(ctx, netKey, info.PortMappings, at); err != nil {
		log.Error("router: persistir redirecciones", "error", err)
	}

	if !hasBaseline {
		if err := st.SetPortMappingsBaselineDone(ctx); err != nil {
			log.Error("router: marcar línea base de redirecciones", "error", err)
		}
		log.Info("router: línea base de redirecciones registrada", "redirecciones", len(info.PortMappings))
		return // EvaluateNewPortMappings ya no generó alertas (hasBaseline era false)
	}
	for _, a := range newAlerts {
		persistAndDispatch(ctx, st, hub, sender, id, a, nil, log)
	}
}

// runOuterRingLoop escanea el anillo exterior cada interval, SIN
// hacerlo también "al arrancar" (a diferencia del interior, ver
// scheduler.Scheduler): verificado contra la red real, ese primer intento
// casi siempre pierde la carrera contra el PRIMER ciclo del anillo
// interior —el que recién AUTODETECTA el anillo exterior vía
// pollRouter→outerRingConfig.observeRouter— porque un core.Scan tarda
// varios segundos y este arrancaría casi en el instante cero; perderla
// dejaría el anillo exterior sin un segundo intento hasta los 10 minutos
// completos. Un ticker liso evita la carrera por construcción: para
// cuando dispare la primera vez, el interior ya tuvo de sobra su primer
// ciclo (inmediato, siempre) para autodetectar. Secuencial a propósito
// (scan, luego record, recién entonces se espera el siguiente tick): el
// límite real de "uno a la vez" entre los dos anillos lo da scanGate
// (compartido con el interior, ver acquireScan en runCmd), no esto. interval
// es un parámetro (en vez de usar outerRingInterval directo) solo para que
// las pruebas no tengan que esperar 10 minutos de verdad.
func runOuterRingLoop(ctx context.Context, interval time.Duration, scan scheduler.ScanFunc, record scheduler.RecordFunc, log *slog.Logger) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			startedAt := time.Now().UTC()
			res, err := scan(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return // apagando: no es una falla real, solo se cortó a mitad
				}
				log.Error("escaneo exterior falló", "error", err)
				continue
			}
			record(ctx, res, startedAt)
		}
	}
}
