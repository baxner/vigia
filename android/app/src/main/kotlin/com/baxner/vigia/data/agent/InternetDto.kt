package com.baxner.vigia.data.agent

import kotlinx.serialization.Serializable

/**
 * Formas JSON de `docs/agente-api.md` § "Internet (v1.4)". Aparte de
 * [AgentDto.kt] (que ya tiene Alertas y Seguridad) porque la sección del
 * contrato es grande por sí sola — medición, caídas, velocidad, ajustes — y
 * no comparte tipos con el resto (ver el mismo criterio que separó
 * `Seguridad` dentro de ese archivo, solo que aquí ya alcanza para su propio
 * archivo).
 */

/** Los tres tramos de la medición más DNS y pérdida (ver el contrato: "Medición"). Cada campo `null` si ese tramo no se pudo medir (p. ej. sin módem conocido). */
@Serializable
data class InternetNowDto(
    val routerMs: Double? = null,
    val modemMs: Double? = null,
    val internetMs: Double? = null,
    val dnsMs: Double? = null,
    val lossPct: Double = 0.0,
)

/** Una caída registrada (ver el contrato § "Caídas"): [end] `null` = sigue en curso. */
@Serializable
data class OutageDto(
    val start: String,
    val end: String? = null,
    val durationSec: Long = 0,
    val blame: String = "unknown",
)

/** `blame` de [OutageDto]: texto crudo en el campo (mismo criterio que [AlertDto.type]), este enum solo para la UI (ver `ui/internet/`). */
enum class OutageBlame { LAN, MODEM, ISP, UNKNOWN }

fun OutageDto.blameOrUnknown(): OutageBlame = when (blame) {
    "lan" -> OutageBlame.LAN
    "modem" -> OutageBlame.MODEM
    "isp" -> OutageBlame.ISP
    else -> OutageBlame.UNKNOWN
}

/** Una prueba de velocidad (ver el contrato § "Prueba de velocidad"): [error] no nulo = la prueba falló (sin bajada/subida que mostrar). */
@Serializable
data class SpeedTestDto(
    val at: String,
    val downMbps: Double? = null,
    val upMbps: Double? = null,
    val latencyMs: Double? = null,
    val loadedLatencyMs: Double? = null,
    val error: String? = null,
)

/** `GET /api/v1/internet` (ver el contrato): el estado de internet calculado por el agente, con lo último de cada medición. */
@Serializable
data class InternetStatusDto(
    val state: String = "unknown",
    val since: String? = null,
    val now: InternetNowDto = InternetNowDto(),
    val ipv6: String = "unknown",
    /** Últimos 30 días, la más nueva primero (ver el contrato). */
    val outages: List<OutageDto> = emptyList(),
    /** Últimas 30 (ver el contrato). */
    val speedTests: List<SpeedTestDto> = emptyList(),
)

/** `state` de [InternetStatusDto]: texto crudo en el campo, este enum solo para la UI. Un valor que esta versión no reconoce cae en [UNKNOWN] (como [AlertType.fromRaw]: mejor no reventar que un agente más nuevo mande un estado nuevo). */
enum class InternetState { UP, DOWN, DEGRADED, UNKNOWN }

fun InternetStatusDto.stateOrUnknown(): InternetState = when (state) {
    "up" -> InternetState.UP
    "down" -> InternetState.DOWN
    "degraded" -> InternetState.DEGRADED
    else -> InternetState.UNKNOWN
}

/** `ipv6` de [InternetStatusDto]: "ok" (sale bien), "broken" (el router lo reparte pero no sale a internet) o "none" (no hay IPv6 en esta red). */
enum class Ipv6State { OK, BROKEN, NONE, UNKNOWN }

fun InternetStatusDto.ipv6OrUnknown(): Ipv6State = when (ipv6) {
    "ok" -> Ipv6State.OK
    "broken" -> Ipv6State.BROKEN
    "none" -> Ipv6State.NONE
    else -> Ipv6State.UNKNOWN
}

/**
 * Un punto de `GET /api/v1/internet/series` (ver el contrato): por minuto
 * (rango `24h`) o por hora (rango `7d`). Cada tramo es `null` donde no hubo
 * dato (el contrato: "`null` donde no hubo dato") — por eso todos nullable,
 * a diferencia de [InternetNowDto] donde `lossPct` siempre viene puesto.
 */
@Serializable
data class InternetSeriesPointDto(
    val at: String,
    val routerMs: Double? = null,
    val modemMs: Double? = null,
    val internetMs: Double? = null,
    val dnsMs: Double? = null,
    val lossPct: Double? = null,
)

/** El rango de `GET /api/v1/internet/series?range=` (ver el contrato): `24h` por minuto, `7d` por hora. */
enum class InternetSeriesRange(val wireValue: String) {
    HOURS_24("24h"),
    DAYS_7("7d"),
}

/** `GET`/`PUT /api/v1/internet/settings` (ver el contrato): pruebas de velocidad automáticas al día, de 0 a 6. */
@Serializable
data class InternetSettingsDto(
    val speedTestsPerDay: Int = 2,
)
