package com.baxner.vigia.ui.internet

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.OutageBlame

/**
 * Formateo puro de la pantalla Internet (ver el encargo, "Caídas": "duración
 * legible... y culpable en palabras"). Sin `Context` ni `stringResource` a
 * propósito — mismo criterio que [com.baxner.vigia.util.formatDurationNanos]:
 * una frase corta y fija, se prueba con JUnit plano sin Robolectric.
 */

/**
 * "3 min", "1 h 20 min", "en curso" (ver el encargo). [ongoing] manda sobre
 * [durationSec] (una caída en curso no tiene una duración final todavía).
 */
fun formatOutageDuration(durationSec: Long, ongoing: Boolean): String {
    if (ongoing) return "en curso"
    val totalMinutes = (durationSec / 60).coerceAtLeast(0)
    val hours = totalMinutes / 60
    val minutes = totalMinutes % 60
    return when {
        hours > 0 && minutes > 0 -> "$hours h $minutes min"
        hours > 0 -> "$hours h"
        minutes > 0 -> "$minutes min"
        else -> "menos de 1 min"
    }
}

/** Versión localizada de [formatOutageDuration] para la UI Compose. */
@Composable
fun localizedOutageDuration(durationSec: Long, ongoing: Boolean): String {
    if (ongoing) return stringResource(R.string.internet_outage_ongoing)
    val totalMinutes = (durationSec / 60).coerceAtLeast(0)
    val hours = totalMinutes / 60
    val minutes = totalMinutes % 60
    return when {
        hours > 0 && minutes > 0 -> stringResource(R.string.internet_outage_hours_minutes, hours, minutes)
        hours > 0 -> stringResource(R.string.internet_outage_hours, hours)
        minutes > 0 -> stringResource(R.string.internet_outage_minutes, minutes)
        else -> stringResource(R.string.internet_outage_less_than_min)
    }
}

/** "Tu router" / "El módem" / "Tu proveedor" / "No se sabe" (ver el encargo, textualmente). */
fun blameLabel(blame: OutageBlame): String = when (blame) {
    OutageBlame.LAN -> "Tu router"
    OutageBlame.MODEM -> "El módem"
    OutageBlame.ISP -> "Tu proveedor"
    OutageBlame.UNKNOWN -> "No se sabe"
}

/** Versión localizada de [blameLabel] para la UI Compose. */
@Composable
fun OutageBlame.localizedLabel(): String = stringResource(
    when (this) {
        OutageBlame.LAN -> R.string.internet_blame_lan
        OutageBlame.MODEM -> R.string.internet_blame_modem
        OutageBlame.ISP -> R.string.internet_blame_isp
        OutageBlame.UNKNOWN -> R.string.internet_blame_unknown
    },
)
