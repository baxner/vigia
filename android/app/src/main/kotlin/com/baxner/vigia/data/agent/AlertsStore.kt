package com.baxner.vigia.data.agent

import android.content.Context
import com.baxner.vigia.data.repository.VigiaJson
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString

/**
 * Lo que la app necesita recordar entre sesiones para las alertas (ver el
 * contrato § "Alertas v1.1" y el encargo, puntos 2 y 4). Separado de la
 * implementación real para poder probar el resto de la lógica (p. ej.
 * [AlertNotificationPlanner], el worker) con un fake en memoria — mismo
 * patrón que [AgentLinkStore]/[EncryptedAgentLinkStore].
 *
 * No va cifrado (a diferencia de [AgentLink]): nada acá es secreto, es un
 * contador y una lista de IPs/ajustes, no un token.
 */
interface AlertsStore {
    /**
     * El id más alto de alerta que esta app ya "vio" (ver el encargo, punto
     * 2: "nunca notifica uno ≤ a ese") — sirve doble: cursor de paginación
     * para `GET /alerts?after=` (punto 4, "última vista") y guarda contra
     * notificar dos veces la misma alerta. 0 si todavía no se vio ninguna.
     */
    fun lastNotifiedAlertId(): Long

    /** No retrocede nunca: una respuesta fuera de orden no debe "desnotificar" lo ya mandado. */
    fun setLastNotifiedAlertId(id: Long)

    /** IPv4 de `/info.addresses` (LAN, Tailscale…), guardadas al vincular/actualizar (ver el encargo, punto 4 y [AgentReachability]). */
    fun agentAddresses(): List<String>
    fun setAgentAddresses(addresses: List<String>)

    /** Último [AlertSettingsDto] visto del agente: para que el worker (sin red extra) y la UI (antes de que responda el `GET`) sepan qué tipos están apagados. Todo activo por defecto. */
    fun cachedSettings(): AlertSettingsDto
    fun setCachedSettings(settings: AlertSettingsDto)

    /** Caché de alertas para la pantalla Alertas (ver el encargo, punto 5): sobrevive a que se cierre la app. Acotada, ver [setCachedAlerts]. */
    fun cachedAlerts(): List<AlertDto>
    fun setCachedAlerts(alerts: List<AlertDto>)

    /**
     * El agente (su `agentId`) del que es todo lo de arriba: los ids de
     * alerta son de cada agente, así que al vincular otro hay que empezar de
     * cero (si no, su alerta 3 quedaría "ya notificada" por la 40 del viejo).
     */
    fun ownerAgentId(): String?
    fun setOwnerAgentId(agentId: String)

    /** Olvida cursor, caché, ajustes y direcciones (no [notificationPermissionAsked]). */
    fun clear()

    /** Si ya se pidió una vez el permiso de notificaciones desde el Panel (ver PanelScreen): no insistir en cada arranque. */
    fun notificationPermissionAsked(): Boolean
    fun setNotificationPermissionAsked()

    /**
     * "Avisos al instante" (ver `docs/plan.md` § "Fase 5.1"): el interruptor
     * de los ajustes del agente, apagado por defecto. Con esto prendido Y la
     * red actual vigilada por el agente ([LiveWatchPolicy.shouldRun]),
     * [LiveWatchService] mantiene la conexión SSE viva con la app cerrada.
     */
    fun liveWatchEnabled(): Boolean
    fun setLiveWatchEnabled(enabled: Boolean)

    /**
     * "Ver tu casa remoto" (ver `AgentSettingsScreen.kt` y el encargo, fase
     * 8): prendido por defecto — fuera de la red de casa, si el agente
     * contesta por Tailscale, [AgentModeController] muestra sus pantallas
     * (Seguridad, Internet, Dispositivos con historial) en vez del escaneo
     * local del teléfono. No es nada secreto ni ligado a un agente en
     * particular (como [liveWatchEnabled]): sobrevive a desvincular y
     * volver a vincular.
     */
    fun remoteViewPreferred(): Boolean
    fun setRemoteViewPreferred(preferred: Boolean)
}

class SharedPrefsAlertsStore(context: Context) : AlertsStore {
    private val prefs = context.applicationContext.getSharedPreferences(PrefsFileName, Context.MODE_PRIVATE)

    override fun lastNotifiedAlertId(): Long = prefs.getLong(KeyLastId, 0L)

    override fun setLastNotifiedAlertId(id: Long) {
        if (id > lastNotifiedAlertId()) prefs.edit().putLong(KeyLastId, id).apply()
    }

    override fun agentAddresses(): List<String> =
        prefs.getString(KeyAddresses, null)?.split(',')?.filter { it.isNotBlank() }.orEmpty()

    override fun setAgentAddresses(addresses: List<String>) {
        prefs.edit().putString(KeyAddresses, addresses.filter { it.isNotBlank() }.joinToString(",")).apply()
    }

    override fun cachedSettings(): AlertSettingsDto =
        prefs.getString(KeySettings, null)
            ?.let { runCatching { VigiaJson.decodeFromString<AlertSettingsDto>(it) }.getOrNull() }
            ?: AlertSettingsDto()

    override fun setCachedSettings(settings: AlertSettingsDto) {
        prefs.edit().putString(KeySettings, VigiaJson.encodeToString(settings)).apply()
    }

    override fun cachedAlerts(): List<AlertDto> =
        prefs.getString(KeyAlerts, null)
            ?.let { runCatching { VigiaJson.decodeFromString<List<AlertDto>>(it) }.getOrNull() }
            .orEmpty()

    override fun setCachedAlerts(alerts: List<AlertDto>) {
        val capped = alerts.sortedByDescending { it.id }.distinctBy { it.id }.take(MaxCachedAlerts)
        prefs.edit().putString(KeyAlerts, VigiaJson.encodeToString(capped)).apply()
    }

    override fun ownerAgentId(): String? = prefs.getString(KeyOwner, null)

    override fun setOwnerAgentId(agentId: String) {
        prefs.edit().putString(KeyOwner, agentId).apply()
    }

    override fun clear() {
        prefs.edit()
            .remove(KeyLastId).remove(KeyAddresses).remove(KeySettings).remove(KeyAlerts).remove(KeyOwner)
            .apply()
    }

    override fun notificationPermissionAsked(): Boolean = prefs.getBoolean(KeyPermissionAsked, false)

    override fun setNotificationPermissionAsked() {
        prefs.edit().putBoolean(KeyPermissionAsked, true).apply()
    }

    override fun liveWatchEnabled(): Boolean = prefs.getBoolean(KeyLiveWatchEnabled, false)

    override fun setLiveWatchEnabled(enabled: Boolean) {
        prefs.edit().putBoolean(KeyLiveWatchEnabled, enabled).apply()
    }

    override fun remoteViewPreferred(): Boolean = prefs.getBoolean(KeyRemoteViewPreferred, true)

    override fun setRemoteViewPreferred(preferred: Boolean) {
        prefs.edit().putBoolean(KeyRemoteViewPreferred, preferred).apply()
    }

    private companion object {
        const val PrefsFileName = "vigia_alerts"
        const val KeyLastId = "lastNotifiedAlertId"
        const val KeyAddresses = "agentAddresses"
        const val KeySettings = "alertSettings"
        const val KeyAlerts = "cachedAlerts"
        const val KeyOwner = "ownerAgentId"
        const val KeyPermissionAsked = "notificationPermissionAsked"
        const val KeyLiveWatchEnabled = "liveWatchEnabled"
        const val KeyRemoteViewPreferred = "remoteViewPreferred"
        const val MaxCachedAlerts = 200
    }
}
