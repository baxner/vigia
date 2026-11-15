package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/baxner/vigia/agent"
	"github.com/baxner/vigia/agent/alerts"
	"github.com/baxner/vigia/agent/api"
	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/store"
	"github.com/baxner/vigia/agent/webhooks"
	"github.com/baxner/vigia/core"
)

// handleHealthTransition evalúa (y persiste) la transición ciego↔normal
// del agente (ver agent/alerts.EvaluateHealth y docs/agente-api.md,
// "AGENT_BLIND"/"AGENT_RECOVERED"). Corre en CADA escaneo, ciego o no:
// recordScan la llama antes de decidir si corta por estar ciego.
func handleHealthTransition(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, netKey string, isBlind bool, warning string, at time.Time, log *slog.Logger) {
	wasBlind, err := st.AgentHealthState(ctx)
	if err != nil {
		log.Error("agent_health: leer", "error", err)
		return
	}
	settings, err := st.AlertSettings(ctx)
	if err != nil {
		log.Error("alert_settings: leer", "error", err)
		settings = alerts.DefaultSettings()
	}

	transitioned, alert := alerts.EvaluateHealth(wasBlind, isBlind, warning, netKey, at, settings)
	if transitioned {
		if err := st.SetAgentHealthState(ctx, isBlind, netKey, at); err != nil {
			log.Error("agent_health: guardar", "error", err)
		}
	}
	if alert == nil {
		return
	}
	persistAndDispatch(ctx, st, hub, sender, id, *alert, nil, log)
}

// persistAndDispatch inserta una alerta ya evaluada (si InsertAlert dice
// que es nueva: ver DedupKey, "una alerta por evento") y, solo si de verdad
// se creó, la publica por SSE y la manda a los webhooks. Factoriza lo que
// handleHealthTransition y las funciones de abajo (processInnerAlerts/
// processOuterAlerts/pollRouter) hacían cada una por su cuenta.
func persistAndDispatch(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, a alerts.Alert, device *history.KnownDevice, log *slog.Logger) {
	persisted, created, err := st.InsertAlert(ctx, a)
	if err != nil {
		log.Error("alerts: insertar", "error", err, "type", a.Type)
		return
	}
	if !created {
		return // reevaluación del mismo evento: ya se avisó
	}
	// DeviceID es *int64: sin desreferenciar, el log mostraba la dirección de memoria.
	var deviceID any = "-"
	if persisted.DeviceID != nil {
		deviceID = *persisted.DeviceID
	}
	log.Info("alerta", "type", persisted.Type, "severity", persisted.Severity, "deviceId", deviceID)
	hub.PublishAlert(persisted)
	dispatchWebhooks(ctx, st, sender, id, persisted, device, log)
}

// emitDeviceAlerts evalúa NEW_DEVICE/IMPORTANT_OFFLINE/IMPORTANT_ONLINE
// (ver agent/alerts.EvaluateScan) sobre los eventos de UN escaneo ya
// registrado y despacha cada alerta nueva. Lo comparten
// processInnerAlerts/processOuterAlerts: la única diferencia entre los dos
// anillos es `ring` (ver agent/alerts.Ring), que solo cambia el texto de
// NEW_DEVICE.
func emitDeviceAlerts(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, netKey string, events []store.RecordedEvent, known []history.KnownDevice, ring alerts.Ring, settings alerts.Settings, log *slog.Logger) {
	if len(events) == 0 {
		return
	}
	baseline := alerts.BaselineOf(known)

	inputs := make([]alerts.EventInput, len(events))
	devicesByID := make(map[int64]history.KnownDevice, len(events))
	for i, ev := range events {
		inputs[i] = alerts.EventInput{Event: ev.Event, Device: ev.Device}
		devicesByID[ev.Device.ID] = ev.Device
	}

	for _, a := range alerts.EvaluateScan(inputs, netKey, baseline, settings, ring) {
		var device *history.KnownDevice
		if a.DeviceID != nil {
			if d, ok := devicesByID[*a.DeviceID]; ok {
				device = &d
			}
		}
		persistAndDispatch(ctx, st, hub, sender, id, a, device, log)
	}
}

// processInnerAlerts aplica TODAS las reglas de Alertas que miran al
// anillo interior tras UN escaneo ya registrado: las de v1.1
// (NEW_DEVICE/IMPORTANT_OFFLINE/IMPORTANT_ONLINE, vía emitDeviceAlerts) y
// las tres de Fase 5 que no nacen de un DeviceEvent (ver
// docs/agente-api.md, "Seguridad v1.2"):
//   - ROUTER_CHANGED: scanned/gatewayIP/prevGatewayMAC/newGatewayMAC vienen
//     de recordScan, que capturó la MAC guardada ANTES de que RecordScan
//     la sobrescribiera (ver agent/alerts.EvaluateRouterChanged).
//   - ARP_CONFLICT: con el mismo `scanned` (el escaneo completo) y los
//     equipos ya conocidos.
//   - RISKY_SERVICE: con los puertos abiertos de los equipos conocidos
//     (ver evaluateRiskyServices, el lado con E/S de la regla pura).
//
// RISKY_SERVICE/ROUTER_CHANGED/ARP_CONFLICT NO aplican al anillo exterior
// (ver processOuterAlerts): por eso son dos funciones separadas en vez de
// una sola con un parámetro "¿interior?" — más claro que una rama interna
// que la mitad de las veces no hace nada.
// justWoke: true si quien llama (recordScan, vía el `scanWake` de
// run.go) detectó un hueco grande desde el escaneo anterior, sospechoso
// de que la máquina estuvo dormida (ver agent/wake y docs/agente-api.md,
// "Suspensión"). Solo afecta a evaluateImportantConnectivity — ver su
// comentario para por qué.
func processInnerAlerts(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, netKey string, events []store.RecordedEvent, scanned []core.Device, gatewayIP, prevGatewayMAC, newGatewayMAC string, at time.Time, justWoke bool, log *slog.Logger) {
	settings, err := st.AlertSettings(ctx)
	if err != nil {
		log.Error("alert_settings: leer", "error", err)
		return
	}
	known, err := st.DevicesByNetwork(ctx, history.NetworkKey(netKey))
	if err != nil {
		log.Error("alerts: leer equipos", "error", err)
		return
	}

	emitDeviceAlerts(ctx, st, hub, sender, id, netKey, events, known, alerts.RingInner, settings, log)
	evaluateImportantConnectivity(ctx, st, hub, sender, id, netKey, known, settings, at, justWoke, log)

	if a := alerts.EvaluateRouterChanged(prevGatewayMAC, newGatewayMAC, netKey, at, settings); a != nil {
		persistAndDispatch(ctx, st, hub, sender, id, *a, nil, log)
	}
	if a := alerts.EvaluateARPConflict(gatewayIP, scanned, known, netKey, at, settings); a != nil {
		persistAndDispatch(ctx, st, hub, sender, id, *a, nil, log)
	}
	for _, a := range evaluateRiskyServices(ctx, st, netKey, known, at, settings, log) {
		var device *history.KnownDevice
		if a.DeviceID != nil {
			if d, ok := deviceByID(known, *a.DeviceID); ok {
				device = &d
			}
		}
		persistAndDispatch(ctx, st, hub, sender, id, a, device, log)
	}
}

// processOuterAlerts aplica solo NEW_DEVICE/IMPORTANT_OFFLINE/
// IMPORTANT_ONLINE a un escaneo del anillo exterior (ver
// docs/plan.md, Fase 5: "NEW_DEVICE también aplica al anillo
// exterior, con su propia línea base"). ROUTER_CHANGED/ARP_CONFLICT/
// RISKY_SERVICE no aplican ahí: los dos primeros son del router del
// anillo interior, y RISKY_SERVICE es explícitamente "un equipo INTERIOR".
func processOuterAlerts(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, netKey string, events []store.RecordedEvent, log *slog.Logger) {
	if len(events) == 0 {
		return
	}
	settings, err := st.AlertSettings(ctx)
	if err != nil {
		log.Error("alert_settings: leer", "error", err)
		return
	}
	known, err := st.DevicesByNetwork(ctx, history.NetworkKey(netKey))
	if err != nil {
		log.Error("alerts: leer equipos", "error", err)
		return
	}
	emitDeviceAlerts(ctx, st, hub, sender, id, netKey, events, known, alerts.RingOuter, settings, log)
}

// evaluateRiskyServices es el lado con E/S de RISKY_SERVICE: lee qué pares
// (equipo, puerto) ya se avisaron alguna vez y si ya hay línea base (ver
// agent/alerts.EvaluateNewRiskyServices), evalúa la regla pura sobre los
// equipos EN LÍNEA de `known` y persiste de una sola pasada los pares
// nuevos — alertados o no, para que la próxima corrida ya no los trate
// como nuevos aunque el puerto se cierre y reabra entre escaneos (ver
// docs/plan.md, Fase 5).
func evaluateRiskyServices(ctx context.Context, st *store.Store, netKey string, known []history.KnownDevice, at time.Time, settings alerts.Settings, log *slog.Logger) []alerts.Alert {
	seen, err := st.SeenRiskyPorts(ctx, netKey)
	if err != nil {
		log.Error("risky_service: leer ya vistos", "error", err)
		return nil
	}
	hasBaseline, err := st.RiskyServicesBaselineDone(ctx)
	if err != nil {
		log.Error("risky_service: leer línea base", "error", err)
		return nil
	}

	inputs := make([]alerts.RiskyDeviceInput, 0, len(known))
	for _, d := range known {
		if !d.Online {
			continue
		}
		inputs = append(inputs, alerts.RiskyDeviceInput{
			DeviceID: d.ID, Name: alerts.DisplayName(d), IP: d.LastIP, OpenPorts: d.Last.OpenPorts,
		})
	}

	newAlerts, newlySeen := alerts.EvaluateNewRiskyServices(inputs, seen, hasBaseline, netKey, at, settings)
	for _, ns := range newlySeen {
		if err := st.RecordRiskyPortSeen(ctx, ns.DeviceID, ns.Port, at); err != nil {
			log.Error("risky_service: guardar", "error", err, "deviceId", ns.DeviceID, "port", ns.Port)
		}
	}
	if !hasBaseline {
		if err := st.SetRiskyServicesBaselineDone(ctx); err != nil {
			log.Error("risky_service: marcar línea base", "error", err)
		}
		log.Info("risky_service: línea base registrada", "pares", len(newlySeen))
		return nil // EvaluateNewRiskyServices ya no devolvió alertas (hasBaseline era false)
	}
	return newAlerts
}

// evaluateImportantConnectivity evalúa y despacha IMPORTANT_OFFLINE/
// IMPORTANT_ONLINE (Fase 5.1, ver agent/alerts.EvaluateImportantOffline/
// EvaluateImportantOnline y docs/plan.md) sobre los equipos importantes de
// known (el anillo interior DESPUÉS de este escaneo). A diferencia de
// RISKY_SERVICE, el estado persistido aquí (agent/store.
// ImportantOfflinePending) no es "para siempre": se limpia en cuanto el
// equipo vuelve a estar en línea, para que el PRÓXIMO episodio pueda
// avisar de nuevo.
//
// justWoke (ver docs/agente-api.md, "Suspensión"): EvaluateImportantOffline
// decide por LastSeen vs. `at` — si la máquina estuvo dormida, LastSeen
// de TODOS los equipos importantes quedó fechado antes de dormir, y
// `at` salta de golpe el hueco completo al despertar. Sin esta
// excepción, el primer escaneo tras despertar vería a cada equipo
// importante como "lleva $hueco sin conexión" y avisaría de todos a la
// vez, aunque ninguno se haya movido — es justo la máquina la que no
// pudo preguntar, no ellos los que se fueron. Se salta SOLO
// EvaluateImportantOffline: si de verdad sigue faltando, el PRÓXIMO
// escaneo (ya sin el hueco de por medio) lo vuelve a evaluar y avisa con
// un ligero retraso, nunca se pierde. EvaluateImportantOnline sigue
// corriendo igual: un equipo que SÍ vuelve a aparecer merece el aviso
// sin excepción, y limpiar su pendiente no depende del hueco.
func evaluateImportantConnectivity(ctx context.Context, st *store.Store, hub *api.Hub, sender *webhooks.Sender, id agent.Identity, netKey string, known []history.KnownDevice, settings alerts.Settings, at time.Time, justWoke bool, log *slog.Logger) {
	pending, err := st.ImportantOfflinePending(ctx)
	if err != nil {
		log.Error("important_offline: leer pendientes", "error", err)
		pending = nil
	}

	if justWoke {
		log.Info("important_offline: hueco grande desde el último escaneo (posible suspensión); no se evalúa este ciclo")
	} else {
		for _, a := range alerts.EvaluateImportantOffline(known, at, netKey, settings) {
			if err := st.SetImportantOfflinePending(ctx, *a.DeviceID); err != nil {
				log.Error("important_offline: marcar pendiente", "error", err, "deviceId", *a.DeviceID)
			}
			var device *history.KnownDevice
			if d, ok := deviceByID(known, *a.DeviceID); ok {
				device = &d
			}
			persistAndDispatch(ctx, st, hub, sender, id, a, device, log)
		}
	}

	onlineAlerts, resolved := alerts.EvaluateImportantOnline(known, pending, at, netKey, settings)
	for _, devID := range resolved {
		if err := st.ClearImportantOfflinePending(ctx, devID); err != nil {
			log.Error("important_offline: limpiar pendiente", "error", err, "deviceId", devID)
		}
	}
	for _, a := range onlineAlerts {
		var device *history.KnownDevice
		if d, ok := deviceByID(known, *a.DeviceID); ok {
			device = &d
		}
		persistAndDispatch(ctx, st, hub, sender, id, a, device, log)
	}
}

// deviceByID busca id en devices (lista corta: los equipos de una red
// casera), para anexar el KnownDevice completo a una alerta antes de
// mandarla a los webhooks (ver docs/agente-api.md, Webhooks: "device":
// KnownDevice|null).
func deviceByID(devices []history.KnownDevice, id int64) (history.KnownDevice, bool) {
	for _, d := range devices {
		if d.ID == id {
			return d, true
		}
	}
	return history.KnownDevice{}, false
}

// dispatchWebhooks manda a cada webhook suscrito (ver store.Webhook.Accepts)
// el payload de la alerta, SIN bloquear a quien llama: cada entrega corre
// en su propia goroutine (ver docs/agente-api.md: "envío asíncrono (no
// bloquea el escaneo)"). ctx es el del ciclo de vida del agente (el mismo
// que recibe el scheduler), no uno por-escaneo: una entrega con
// reintentos puede seguir corriendo bastante después de que recordScan ya
// volvió.
func dispatchWebhooks(ctx context.Context, st *store.Store, sender *webhooks.Sender, id agent.Identity, a store.Alert, device *history.KnownDevice, log *slog.Logger) {
	hooks, err := st.Webhooks(ctx)
	if err != nil {
		log.Error("webhooks: listar", "error", err)
		return
	}
	for _, h := range hooks {
		if !h.Accepts(string(a.Type)) {
			continue
		}
		go deliverWebhook(ctx, st, sender, id, h, a, device, log)
	}
}

func deliverWebhook(ctx context.Context, st *store.Store, sender *webhooks.Sender, id agent.Identity, h store.Webhook, a store.Alert, device *history.KnownDevice, log *slog.Logger) {
	body, err := json.Marshal(webhooks.Payload{
		Event: "alert", Alert: a, Device: device,
		Agent: webhooks.AgentInfo{ID: id.ID, Name: id.Name},
	})
	if err != nil {
		log.Error("webhooks: codificar payload", "error", err, "webhook", h.ID)
		return
	}

	res := sender.Deliver(ctx, webhooks.Target{ID: h.ID, URL: h.URL, Secret: h.Secret}, body, string(a.Type))
	var errMsg *string
	if res.Err != nil {
		m := res.Err.Error()
		errMsg = &m
	}
	if err := st.UpdateWebhookResult(ctx, h.ID, res.Status, errMsg); err != nil {
		log.Error("webhooks: guardar resultado", "error", err, "webhook", h.ID)
	}
	if !res.Success() {
		log.Warn("webhooks: entrega sin éxito", "webhook", h.ID, "url", h.URL, "status", res.Status, "error", res.Err)
	}
}
