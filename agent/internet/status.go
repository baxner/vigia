package internet

import "sort"

// EvaluateState decide el estado agregado (ver docs/agente-api.md,
// InternetStatus.state: "degraded: pérdida > 5 % o latencia > 3× la
// mediana de 24 h"):
//   - down: hay una caída abierta AHORA (outageOpen) — manda sobre
//     cualquier otra cosa, aunque la pérdida del último minuto todavía no
//     llegue al 5 % (2 fallas seguidas ya confirmaron la caída).
//   - degraded: sin caída abierta, pero currentLossPct > 5, o
//     currentInternetMs > 3×median24hMs (median24hMs <= 0 significa "sin
//     suficiente historia todavía" y por seguridad nunca degrada por este
//     criterio solo).
//   - up: ninguna de las anteriores.
func EvaluateState(outageOpen bool, currentLossPct, currentInternetMs, median24hMs float64) State {
	if outageOpen {
		return StateDown
	}
	if currentLossPct > 5 {
		return StateDegraded
	}
	if median24hMs > 0 && currentInternetMs > 3*median24hMs {
		return StateDegraded
	}
	return StateUp
}

// ClassifyIPv6 decide IPv6OK/IPv6Broken/IPv6None (ver docs/agente-api.md,
// "IPv6"): hasGlobal es si el host tiene una dirección IPv6 global
// asignada (SLAAC/DHCPv6 — ver cmd/vigia-agent, hasGlobalIPv6); dialOK es
// si la conexión de prueba a [2606:4700:4700::1111]:443 contestó. Sin
// dirección global no tiene sentido intentar la conexión (ver
// docs/agente-api.md: "si el host tiene IPv6 global, se prueba salir por
// IPv6"), así que dialOK se ignora cuando hasGlobal es false.
func ClassifyIPv6(hasGlobal, dialOK bool) IPv6Status {
	if !hasGlobal {
		return IPv6None
	}
	if dialOK {
		return IPv6OK
	}
	return IPv6Broken
}

// Median calcula la mediana de una lista de latencias (ms) ya filtrada a
// las muestras con dato real (ver SegmentStats.HasLatency): 0 si vals está
// vacía, para que EvaluateState la trate como "sin historia todavía". No
// modifica vals (trabaja sobre una copia).
func Median(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	mid := n / 2
	if n%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}
