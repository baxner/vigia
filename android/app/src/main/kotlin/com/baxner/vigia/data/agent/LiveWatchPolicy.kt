package com.baxner.vigia.data.agent

/** Qué dice la notificación fija de [LiveWatchService] (ver [LiveWatchPolicy.notificationKindFor]). */
enum class LiveWatchNotificationKind {
    /** En la red de casa, vigilando de verdad: "Vigía vigilando · N equipos". */
    WATCHING,

    /** Vinculado pero fuera de esa red, en espera de volver: "Vigía · fuera de casa, en espera". */
    AWAY_WAITING,

    /** Vinculado, en (lo que debería ser) su red, pero el agente no contestó: "Vigía · sin conexión con el agente". */
    AGENT_UNREACHABLE,
}

/**
 * "Avisos al instante" (ver `docs/plan.md` § "Fase 5.1"): cuándo debe estar
 * vivo [LiveWatchService] y qué debe decir su notificación. Pura — sin
 * Android, sin red — para poder probarla con JUnit plano; quien de verdad
 * arranca/detiene el servicio es [AgentModeController] (ver `syncLiveWatch`)
 * y, desde dentro del propio servicio, su reevaluación al cambiar de red.
 */
object LiveWatchPolicy {
    /**
     * Debe correr con el interruptor prendido ([enabled]) y un agente
     * vinculado ([AgentPanelState.Linked]) — **no** hace falta que la red
     * actual sea la de casa (ver [AgentPanelState.Linked.monitoringCurrentNetwork]):
     * revisión de diseño del orquestador, para que el servicio sobreviva
     * fuera de casa (sin SSE — [AgentModeController.evaluate] ya cierra esa
     * sesión cuando la red no coincide — solo con su
     * [android.net.ConnectivityManager.NetworkCallback], que no gasta) y
     * pueda reanudar la conexión en vivo solo al volver, sin que el usuario tenga
     * que abrir la app. Sin vínculo ([AgentPanelState.NotLinked]) no hay
     * nada que vigilar: el sondeo de [com.baxner.vigia.work.AlertsSyncWorker]
     * sigue cubriendo las alertas cada ~15 min.
     */
    fun shouldRun(enabled: Boolean, panelState: AgentPanelState): Boolean =
        enabled && panelState is AgentPanelState.Linked

    /**
     * Solo tiene sentido llamarla cuando [shouldRun] ya dio `true` (un
     * [AgentPanelState.Linked]); con [AgentPanelState.NotLinked] el
     * servicio ya se detuvo y esto no debería evaluarse — cae en
     * [LiveWatchNotificationKind.AWAY_WAITING] por default, para no lanzar.
     */
    fun notificationKindFor(panelState: AgentPanelState): LiveWatchNotificationKind = when {
        panelState !is AgentPanelState.Linked -> LiveWatchNotificationKind.AWAY_WAITING
        !panelState.reachable -> LiveWatchNotificationKind.AGENT_UNREACHABLE
        panelState.monitoringCurrentNetwork -> LiveWatchNotificationKind.WATCHING
        else -> LiveWatchNotificationKind.AWAY_WAITING
    }
}
