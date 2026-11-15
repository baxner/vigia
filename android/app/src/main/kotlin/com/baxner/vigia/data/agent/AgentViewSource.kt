package com.baxner.vigia.data.agent

/**
 * Qué fuente de datos debe mostrar la app para Dispositivos/Timeline,
 * Seguridad e Internet (ver el encargo, fase 8: "todas las pantallas fuera
 * de casa por Tailscale, no solo las alertas").
 */
sealed interface AgentViewSource {
    /** Red actual, escaneo del teléfono (sin agente, o agente vinculado pero sin usarlo ahora). */
    data object CurrentNetwork : AgentViewSource

    /** La red actual del teléfono ES la de casa: igual que siempre ("Red monitoreada"), por la LAN. */
    data object Monitored : AgentViewSource

    /** La casa, remoto por Tailscale: el teléfono está en otra red pero el agente contestó por fuera de la LAN. */
    data object Remote : AgentViewSource
}

/**
 * Decide [AgentViewSource] a partir de lo que ya averiguó
 * [AgentModeController.evaluate]: si la red actual es la de casa
 * ([localMatch], ver [AgentModeSelector.matchingNetwork]), si el agente
 * contestó remoto ([remoteReachable], ver [resolvePinnedReachableAgent]) y
 * si el usuario quiere verlo así cuando se pueda ([remoteViewPreferred], ver
 * `AgentSettingsScreen.kt`). Pura — sin red, sin Android — para poder
 * probarla con JUnit plano.
 *
 * [localMatch] manda siempre: si la red actual ya es la de casa no hay nada
 * que decidir (ni tiene sentido haber probado remoto — [AgentModeController]
 * no lo hace en ese caso). Sin eso, [remoteReachable] y la preferencia del
 * usuario deciden entre ver la casa remoto o quedarse con el escaneo local
 * del teléfono.
 */
object AgentViewSourceSelector {
    fun decide(
        localMatch: Boolean,
        remoteReachable: Boolean,
        remoteViewPreferred: Boolean,
    ): AgentViewSource = when {
        localMatch -> AgentViewSource.Monitored
        remoteReachable && remoteViewPreferred -> AgentViewSource.Remote
        else -> AgentViewSource.CurrentNetwork
    }
}
