package com.baxner.vigia.util

import java.time.Duration
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException
import java.util.Locale

// java.time está disponible nativo desde API 26 (nuestro minSdk): no hace
// falta core library desugaring para usarlo.

/**
 * Hora local legible ("10:14:49 p.m." o "22:14:49") de un timestamp RFC3339 del
 * motor. El motor Go en Android no conoce la zona del teléfono y manda UTC:
 * aquí se pasa a la zona local. Si no se puede parsear, devuelve el texto
 * crudo en vez de reventar.
 */
fun formatClockTime(
    rfc3339: String,
    is24Hour: Boolean,
    zone: ZoneId = ZoneId.systemDefault(),
    locale: Locale = Locale.getDefault(),
): String {
    if (rfc3339.isBlank()) return ""
    // El formato de 12/24 h lo decide el ajuste del teléfono, no el idioma.
    val pattern = if (is24Hour) "HH:mm:ss" else "h:mm:ss a"
    return try {
        OffsetDateTime.parse(rfc3339)
            .atZoneSameInstant(zone)
            .format(DateTimeFormatter.ofPattern(pattern, locale))
    } catch (_: DateTimeParseException) {
        rfc3339
    }
}

/** Nanosegundos (como manda Go) a texto corto, p. ej. "3.4 s". */
fun formatDurationNanos(nanos: Long): String {
    val seconds = nanos / 1_000_000_000.0
    return "%.1f s".format(seconds)
}

/**
 * Cuánto hace un timestamp, como unidad+cantidad: la UI decide el texto
 * exacto ("hace 2 min") con sus propios recursos de string. Separado de la
 * UI para poder probarlo con JUnit plano, igual que el resto de este archivo.
 */
sealed interface RelativeTime {
    data object JustNow : RelativeTime
    data class Seconds(val value: Long) : RelativeTime
    data class Minutes(val value: Long) : RelativeTime
    data class Hours(val value: Long) : RelativeTime
    /** 24 h o más (ver la revisión de la fase 5: "Desde hace 144 h" no es legible — una redirección UPnP de varios días, un router sin cambiar en semanas…). */
    data class Days(val value: Long) : RelativeTime
}

/**
 * [RelativeTime] de [rfc3339] respecto de [now]. Null si [rfc3339] viene
 * vacío o no se pudo parsear (p. ej. todavía no hay un escaneo).
 */
fun relativeTimeFrom(rfc3339: String, now: OffsetDateTime = OffsetDateTime.now()): RelativeTime? {
    if (rfc3339.isBlank()) return null
    val then = try {
        OffsetDateTime.parse(rfc3339)
    } catch (_: DateTimeParseException) {
        return null
    }
    return relativeTimeBetween(then.toInstant(), now.toInstant())
}

/**
 * Igual que [relativeTimeFrom] pero para los timestamps del historial local
 * ([java.time.Instant], no texto RFC3339 del motor): nunca falla al parsear,
 * así que no hace falta que devuelva nulo.
 */
fun relativeTimeFrom(instant: Instant, now: Instant = Instant.now()): RelativeTime =
    relativeTimeBetween(instant, now)

private fun relativeTimeBetween(then: Instant, now: Instant): RelativeTime {
    val seconds = Duration.between(then, now).seconds.coerceAtLeast(0)
    return when {
        seconds < 45 -> RelativeTime.JustNow
        seconds < 60 -> RelativeTime.Seconds(seconds)
        seconds < 3600 -> RelativeTime.Minutes(seconds / 60)
        seconds < 86_400 -> RelativeTime.Hours(seconds / 3600)
        else -> RelativeTime.Days(seconds / 86_400)
    }
}

/**
 * Fecha y hora local ("3 oct 2026 · 10:14" o "3 oct 2026 · 10:14 p.m.") de un
 * [Instant] del historial local: para "Visto por primera vez", "En línea
 * desde" y cada fila de la cronología, donde a diferencia del reloj en vivo
 * del escaneo sí puede importar de qué día se trata.
 */
fun formatDateTime(
    instant: Instant,
    is24Hour: Boolean,
    zone: ZoneId = ZoneId.systemDefault(),
    locale: Locale = Locale.getDefault(),
): String {
    val timePattern = if (is24Hour) "HH:mm" else "h:mm a"
    return DateTimeFormatter.ofPattern("d MMM yyyy '·' $timePattern", locale)
        .withZone(zone)
        .format(instant)
}
