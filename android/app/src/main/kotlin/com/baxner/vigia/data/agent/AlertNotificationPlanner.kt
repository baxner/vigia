package com.baxner.vigia.data.agent

import java.time.Duration
import java.time.Instant
import java.time.OffsetDateTime

/**
 * La regla pura de "qué notificar" (ver el encargo, puntos 2 y 6: "sin
 * duplicados, orden, canal por tipo"): sin Android, sin red, sin horario —
 * solo listas y el [AlertSettingsDto] vigente. La usan tanto el camino en
 * vivo (SSE, ver `AlertsRepository`) como el de la app cerrada
 * ([com.baxner.vigia.work.AlertsSyncWorker]), para que las dos rutas decidan
 * exactamente igual.
 */
object AlertNotificationPlanner {
    /**
     * Una alerta más vieja que esto queda en la lista pero ya no se notifica:
     * al vincular por primera vez (cursor en 0) o tras días con el teléfono
     * apagado, `GET /alerts?after=` trae todo lo pendiente, y soltarlo de
     * golpe como avisos sería ruido. Un día cubre lo que pasó de noche con el
     * teléfono en reposo profundo (WorkManager puede tardar horas entonces).
     */
    val MaxNotifyAge: Duration = Duration.ofHours(24)

    /**
     * De [incoming] (alertas recién vistas, en cualquier orden y con
     * posibles repetidos), cuáles mostrar como notificación y en qué orden:
     * solo las que tengan `id > [lastNotifiedId]` (ver el encargo, punto 2:
     * "nunca notifica uno ≤ a ese") y cuyo tipo esté activo en [settings],
     * de la más vieja a la más nueva (para que la más reciente quede arriba
     * en la bandeja del sistema). Las de antes de [MaxNotifyAge] respecto de
     * [now] no se notifican.
     *
     * Un tipo que esta versión de la app no reconoce (`AlertType.fromRaw`
     * devuelve null — un tipo nuevo de un agente más nuevo) se notifica
     * igual: no hay ajuste para apagarlo, y mejor pasar de más que perder
     * una alerta real (ver `channelId()` en `notifications/AlertNotifications.kt`,
     * que le da el canal de menor prioridad por el mismo motivo).
     */
    fun plan(
        incoming: List<AlertDto>,
        lastNotifiedId: Long,
        settings: AlertSettingsDto,
        now: Instant = Instant.now(),
    ): List<AlertDto> =
        incoming
            .distinctBy { it.id }
            .filter { it.id > lastNotifiedId }
            .filter { alert -> AlertType.fromRaw(alert.type)?.let(settings::isEnabled) ?: true }
            .filter { alert -> isRecent(alert, now) }
            .sortedBy { it.id }

    /** Sin fecha legible se notifica igual: mejor pasar de más que perder una alerta real (mismo criterio que con un tipo desconocido). */
    private fun isRecent(alert: AlertDto, now: Instant): Boolean {
        val at = runCatching { OffsetDateTime.parse(alert.at).toInstant() }.getOrNull() ?: return true
        return !at.isBefore(now.minus(MaxNotifyAge))
    }

    /**
     * El próximo cursor a guardar en [AlertsStore.setLastNotifiedAlertId]:
     * avanza al id más alto visto en [seen] (TODO lo que llegó del agente,
     * no solo lo [plan]eado — una alerta de un tipo apagado igual se "vio",
     * y no hay que volver a pedirla la próxima vez). Si [seen] viene vacío,
     * se queda en [previous].
     */
    fun nextCursor(seen: List<AlertDto>, previous: Long): Long = seen.maxOfOrNull { it.id } ?: previous
}
