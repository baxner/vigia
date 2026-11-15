// Package scheduler corre core.Scan cada cierto intervalo y, a pedido, de
// inmediato (POST /api/v1/scan): "uno a la vez" — un escaneo manual nunca
// se superpone con uno periódico ni con otro manual, el guard es el mismo
// para los dos caminos (ver docs/agente-api.md, sección scheduler).
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/baxner/vigia/agent/wake"
	"github.com/baxner/vigia/core"
)

// DefaultInterval: valor por defecto del contrato ("un escaneo cada 5 min").
const DefaultInterval = 5 * time.Minute

// wakePollInterval: cada cuánto se revisa si hubo un hueco de suspensión
// (ver checkWake y docs/agente-api.md, "Suspensión"). Deliberadamente
// chico y fijo (no depende de `interval`): es lo que determina qué tan
// pronto, como mucho, se nota que la máquina despertó — no el intervalo
// de escaneo configurado, que puede ser de horas si alguien lo pone así.
const wakePollInterval = 10 * time.Second

// wakeFactor: el hueco entre dos revisiones de wakePollInterval tiene que
// superar `interval × wakeFactor` para sospechar suspensión (ver
// agent/wake.Detector). 2.0 porque un escaneo que de verdad tardó un poco
// más de lo normal nunca llega a duplicar su propio intervalo, y porque
// hay que quedar cómodamente por debajo del umbral de 15 min con que
// Alertas decide "equipo importante fuera de línea" — así cualquier hueco
// capaz de disparar esa alerta sin ser suspensión real ya se habría
// detectado aquí primero.
const wakeFactor = 2.0

// ScanFunc ejecuta un escaneo —normalmente core.Scan con las opciones ya
// fijas del agente (ver cmd/vigia-agent)—. Inyectable para probar el
// scheduler sin tocar la red real.
type ScanFunc func(ctx context.Context) (*core.ScanResult, error)

// RecordFunc procesa un ScanResult ya terminado: reconciliarlo contra el
// store y publicar lo que corresponda en el stream SSE. at es cuándo
// EMPEZÓ el escaneo (no cuándo terminó) — es lo que queda como lastSeen de
// los equipos y lastScanAt de la red, igual en todo el agente.
type RecordFunc func(ctx context.Context, res *core.ScanResult, at time.Time)

// Scheduler dispara escaneos periódicos y, a pedido, inmediatos.
//
// Vive mientras dure el proceso: el contexto que recibe en New es el del
// ciclo de vida del agente (cancelado al apagar), NO el de un request HTTP
// puntual — un escaneo disparado por POST /api/v1/scan sigue corriendo en
// segundo plano después de que esa respuesta 202 ya se mandó, así que no
// puede depender del contexto de ESE request (se cancelaría apenas
// terminara de escribirse la respuesta).
type Scheduler struct {
	ctx      context.Context
	scan     ScanFunc
	record   RecordFunc
	interval time.Duration
	log      *slog.Logger
	wake     *wake.Detector

	mu       sync.Mutex
	inFlight bool

	lastMu     sync.RWMutex
	lastScanAt time.Time
}

// New crea un Scheduler atado a ctx. interval <= 0 usa DefaultInterval.
// log nil usa slog.Default().
func New(ctx context.Context, scan ScanFunc, record RecordFunc, interval time.Duration, log *slog.Logger) *Scheduler {
	if interval <= 0 {
		interval = DefaultInterval
	}
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{ctx: ctx, scan: scan, record: record, interval: interval, log: log, wake: wake.NewDetector(interval, wakeFactor)}
}

// Interval devuelve el intervalo configurado (ver GET /api/v1/info,
// "scanIntervalSec") — una sola fuente de verdad en vez de que quien
// construye el Scheduler tenga que recordarlo aparte.
func (s *Scheduler) Interval() time.Duration { return s.interval }

// LastScanAt devuelve cuándo EMPEZÓ el último escaneo que terminó (cero si
// ninguno terminó todavía). Para GET /api/v1/info.
func (s *Scheduler) LastScanAt() time.Time {
	s.lastMu.RLock()
	defer s.lastMu.RUnlock()
	return s.lastScanAt
}

// Run dispara un escaneo al arrancar y luego uno cada interval, hasta que
// el contexto de New se cancele. Bloquea: quien llama lo corre en su propia
// goroutine (ver cmd/vigia-agent run).
//
// En paralelo, un segundo ticker (wakePollInterval, mucho más corto que
// interval) vigila si la máquina se durmió (ver checkWake): sin esto, un
// escaneo pendiente podría tardar hasta un `interval` completo en
// notarse tras despertar, porque el time.Ticker principal cuenta el
// tiempo que YA había transcurrido antes de dormir y no hay garantía de
// que la plataforma lo haga avanzar durante la suspensión igual que el
// reloj de pared (ver docs/agente-api.md, "Suspensión").
func (s *Scheduler) Run() {
	s.startAsync()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	wakeTicker := time.NewTicker(wakePollInterval)
	defer wakeTicker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.startAsync()
		case now := <-wakeTicker.C:
			s.checkWake(now)
		}
	}
}

// checkWake alimenta el detector de suspensión con now y, si el hueco
// contra la última llamada es sospechoso (ver agent/wake.Detector),
// dispara un escaneo de inmediato en vez de esperar al próximo tick del
// ticker principal. Separado de Run para que las pruebas lo llamen con
// tiempos fijos, sin depender de wakePollInterval real.
func (s *Scheduler) checkWake(now time.Time) {
	if asleep, gap := s.wake.Feed(now); asleep {
		s.log.Warn("scheduler: hueco grande entre revisiones, posible suspensión de la máquina; escaneo inmediato", "hueco", gap)
		s.startAsync()
	}
}

// TriggerNow pide un escaneo inmediato (POST /api/v1/scan). Devuelve false
// si ya hay uno en curso —periódico o manual—, para que el handler HTTP
// responda 409 en vez de encolar uno nuevo.
func (s *Scheduler) TriggerNow() bool {
	return s.startAsync()
}

func (s *Scheduler) startAsync() bool {
	s.mu.Lock()
	if s.inFlight {
		s.mu.Unlock()
		return false
	}
	s.inFlight = true
	s.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("escaneo: panic recuperado", "panic", r)
			}
			s.mu.Lock()
			s.inFlight = false
			s.mu.Unlock()
		}()
		s.execute()
	}()
	return true
}

func (s *Scheduler) execute() {
	startedAt := time.Now().UTC()
	res, err := s.scan(s.ctx)
	if err != nil {
		if s.ctx.Err() != nil {
			return // apagando: no es una falla real, solo se cortó a mitad.
		}
		s.log.Error("escaneo falló", "error", err)
		return
	}
	if s.record != nil {
		s.record(s.ctx, res, startedAt)
	}
	s.lastMu.Lock()
	s.lastScanAt = startedAt
	s.lastMu.Unlock()
}
