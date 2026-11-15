package com.baxner.vigia.data.widget

import com.baxner.vigia.data.agent.InternetState
import java.time.Instant
import kotlinx.serialization.Serializable

/**
 * Lo que necesita el widget de pantalla de inicio (fase 8, ver `docs/plan.md`
 * y `ui/widget/VigiaWidget.kt`): equipos en línea, alertas sin leer, estado
 * de internet y hace cuánto se actualizó. Un resumen chico, no la lista
 * completa de equipos/alertas — el widget no necesita más, y así cabe
 * entero en `SharedPreferences` (ver [WidgetSummaryStore]).
 *
 * Siempre sale de lo que la app ya tiene en memoria al momento de refrescar
 * (ver el encargo: "Los datos salen de lo que ya hay en caché"), nunca de
 * una llamada de red propia — por eso todas las actualizaciones de este
 * archivo son funciones puras: solo combinan un [WidgetSummaryDto] anterior
 * con el dato nuevo, sin tocar nada de Android (ver las pruebas en
 * `WidgetSummaryTest.kt`).
 */
@Serializable
data class WidgetSummaryDto(
    /** Agente vinculado (ver [com.baxner.vigia.data.agent.AgentModeController.isLinked]). Sin esto, el widget solo muestra "Vincula el agente" (ver el encargo). */
    val linked: Boolean = false,
    val onlineDevices: Int = 0,
    val unreadAlerts: Int = 0,
    /** "up" | "degraded" | "down" | "none" (ver [toWidgetWireState]); "none" = sin dato todavía, o esta red no la vigila el agente. */
    val internetState: String = "none",
    /** `null` si [internetState] no trae una medición con latencia (p. ej. "down" o "none"). */
    val internetLatencyMs: Int? = null,
    /** Epoch-millis de la última vez que se actualizó cualquier parte de este resumen; `null` = nunca. */
    val updatedAtEpochMs: Long? = null,
)

/**
 * "Vincula el agente" (ver el encargo): sin vínculo no hay nada que mostrar,
 * así que se descarta todo lo demás (no tiene sentido dejar "3 equipos" de
 * la sesión anterior colgado debajo del mensaje de "vincula el agente"). Con
 * [linked] `true`, solo marca el campo y el reloj: cada pieza (equipos,
 * alertas, internet) llega después por su propio [withOnlineDevices]/etc.
 */
fun WidgetSummaryDto.withLinked(linked: Boolean, now: Instant): WidgetSummaryDto =
    if (linked) copy(linked = true, updatedAtEpochMs = now.toEpochMilli())
    else WidgetSummaryDto(linked = false, updatedAtEpochMs = now.toEpochMilli())

fun WidgetSummaryDto.withOnlineDevices(onlineDevices: Int, now: Instant): WidgetSummaryDto =
    copy(onlineDevices = onlineDevices, updatedAtEpochMs = now.toEpochMilli())

fun WidgetSummaryDto.withUnreadAlerts(unreadAlerts: Int, now: Instant): WidgetSummaryDto =
    copy(unreadAlerts = unreadAlerts, updatedAtEpochMs = now.toEpochMilli())

fun WidgetSummaryDto.withInternet(state: InternetState, latencyMs: Int?, now: Instant): WidgetSummaryDto =
    copy(internetState = state.toWidgetWireState(), internetLatencyMs = latencyMs, updatedAtEpochMs = now.toEpochMilli())

/** Texto crudo guardado en [WidgetSummaryDto.internetState]: mismo criterio que el resto del cliente del agente (ver `AgentDto.kt`), un valor sin reconocer cae en "none". */
fun InternetState.toWidgetWireState(): String = when (this) {
    InternetState.UP -> "up"
    InternetState.DEGRADED -> "degraded"
    InternetState.DOWN -> "down"
    InternetState.UNKNOWN -> "none"
}
