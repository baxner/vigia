package com.baxner.vigia.ui.widget

import android.content.Context
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.GlanceId
import androidx.glance.GlanceModifier
import androidx.glance.LocalContext
import androidx.glance.LocalSize
import androidx.glance.action.actionStartActivity
import androidx.glance.action.clickable
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.SizeMode
import androidx.glance.appwidget.cornerRadius
import androidx.glance.appwidget.provideContent
import androidx.glance.background
import androidx.glance.layout.Alignment
import androidx.glance.layout.Box
import androidx.glance.layout.Column
import androidx.glance.layout.Row
import androidx.glance.layout.fillMaxSize
import androidx.glance.layout.fillMaxWidth
import androidx.glance.layout.padding
import androidx.glance.text.FontWeight
import androidx.glance.text.Text
import androidx.glance.text.TextAlign
import androidx.glance.text.TextStyle
import androidx.glance.unit.ColorProvider
import com.baxner.vigia.MainActivity
import com.baxner.vigia.R
import com.baxner.vigia.data.widget.SharedPrefsWidgetSummaryStore
import com.baxner.vigia.data.widget.WidgetSummaryDto
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.util.RelativeTime
import com.baxner.vigia.util.relativeTimeFrom
import java.time.Instant

/**
 * Widget de pantalla de inicio (fase 8, ver `docs/plan.md`): equipos en
 * línea, alertas sin leer, estado de internet y hace cuánto se actualizó —
 * el mismo resumen de la tarjeta "Vigilancia 24/7" del Panel (ver
 * `ui/agent/AgentCard.kt`), pero sin abrir la app. Tocarlo sí la abre.
 *
 * Lee [com.baxner.vigia.data.widget.WidgetSummaryStore] directo, una sola
 * vez por redibujado: sin red ni sondeo propios (ver el encargo). Quien lo
 * mantiene al día es [com.baxner.vigia.data.widget.WidgetSummaryUpdater],
 * llamado desde `AlertsSyncWorker` y desde la app abierta (ver
 * `ScanViewModel`/`AlertsRepository`/`InternetViewModel`) — nunca más
 * seguido que cada 15 min por sí solo (ver `vigia_widget_info.xml`:
 * `updatePeriodMillis="0"`, el reloj de verdad es el de `AlertsSyncWorker`).
 */
class VigiaWidget : GlanceAppWidget() {
    // Los dos tamaños "de catálogo" que pide el encargo (2x1 y 3x2); Glance
    // elige el que no sobrepase el espacio que el usuario le dio en su
    // pantalla de inicio.
    override val sizeMode = SizeMode.Responsive(setOf(CompactSize, ExpandedSize))

    override suspend fun provideGlance(context: Context, id: GlanceId) {
        val summary = SharedPrefsWidgetSummaryStore(context).get()
        provideContent {
            WidgetContent(summary)
        }
    }

    companion object {
        val CompactSize = DpSize(110.dp, 40.dp)
        val ExpandedSize = DpSize(180.dp, 110.dp)
    }
}

@Composable
private fun WidgetContent(summary: WidgetSummaryDto) {
    val context = LocalContext.current
    Box(
        modifier = GlanceModifier
            .fillMaxSize()
            .background(VigiaColors.Surface)
            .cornerRadius(16.dp)
            .padding(12.dp)
            .clickable(actionStartActivity<MainActivity>()),
        contentAlignment = Alignment.CenterStart,
    ) {
        if (!summary.linked) {
            Text(
                text = context.getString(R.string.widget_not_linked),
                style = widgetTextStyle(VigiaColors.TextSecondary, 13.sp),
            )
        } else if (LocalSize.current.width < VigiaWidget.ExpandedSize.width) {
            CompactContent(context, summary)
        } else {
            ExpandedContent(context, summary)
        }
    }
}

/** 2x1: una sola fila, lo más corto que cabe (ver el encargo: "tamaños 2x1 y 3x2 al menos"). */
@Composable
private fun CompactContent(context: Context, summary: WidgetSummaryDto) {
    Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = context.getString(R.string.widget_devices_short, summary.onlineDevices),
            style = widgetTextStyle(VigiaColors.TextPrimary, 13.sp, FontWeight.Bold),
            modifier = GlanceModifier.padding(end = 10.dp),
        )
        Text(
            text = internetShortLabel(context, summary),
            style = widgetTextStyle(internetColor(summary), 12.sp),
            modifier = GlanceModifier.padding(end = 10.dp),
        )
        if (summary.unreadAlerts > 0) {
            Text(
                text = context.getString(R.string.widget_alerts_short, summary.unreadAlerts),
                style = widgetTextStyle(VigiaColors.Risk, 12.sp, FontWeight.Bold),
            )
        }
    }
}

/** 3x2 (o más): una fila por dato, con etiqueta completa y "hace cuánto" abajo. */
@Composable
private fun ExpandedContent(context: Context, summary: WidgetSummaryDto) {
    Column(modifier = GlanceModifier.fillMaxWidth()) {
        Text(
            text = context.getString(R.string.widget_title),
            style = widgetTextStyle(VigiaColors.RadarCyan, 13.sp, FontWeight.Bold),
            modifier = GlanceModifier.padding(bottom = 6.dp),
        )
        StatRow(context.getString(R.string.widget_devices_label, summary.onlineDevices), VigiaColors.TextPrimary)
        StatRow(context.getString(R.string.widget_alerts_label, summary.unreadAlerts), if (summary.unreadAlerts > 0) VigiaColors.Risk else VigiaColors.TextPrimary)
        StatRow(internetFullLabel(context, summary), internetColor(summary))
        Text(
            text = agoText(context, summary.updatedAtEpochMs),
            style = widgetTextStyle(VigiaColors.TextTertiary, 10.sp),
            modifier = GlanceModifier.padding(top = 6.dp),
        )
    }
}

@Composable
private fun StatRow(text: String, color: androidx.compose.ui.graphics.Color) {
    Text(text = text, style = widgetTextStyle(color, 12.sp), modifier = GlanceModifier.padding(bottom = 2.dp))
}

private fun widgetTextStyle(
    color: androidx.compose.ui.graphics.Color,
    fontSize: androidx.compose.ui.unit.TextUnit,
    fontWeight: androidx.glance.text.FontWeight = FontWeight.Normal,
): TextStyle = TextStyle(color = ColorProvider(color), fontSize = fontSize, fontWeight = fontWeight, textAlign = TextAlign.Start)

private fun internetColor(summary: WidgetSummaryDto): androidx.compose.ui.graphics.Color = when (summary.internetState) {
    "up" -> VigiaColors.Ok
    "degraded" -> VigiaColors.Amber
    "down" -> VigiaColors.Risk
    else -> VigiaColors.TextTertiary
}

/** "bien · 18 ms" / "sin internet" (ver el encargo, textual). */
private fun internetFullLabel(context: Context, summary: WidgetSummaryDto): String {
    val stateLabel = internetStateLabel(context, summary.internetState)
    val latency = summary.internetLatencyMs
    return if (latency != null) context.getString(R.string.widget_internet_with_latency, stateLabel, latency) else stateLabel
}

private fun internetShortLabel(context: Context, summary: WidgetSummaryDto): String {
    val latency = summary.internetLatencyMs
    return if (summary.internetState == "up" || summary.internetState == "degraded") {
        latency?.let { context.getString(R.string.widget_internet_ms_short, it) } ?: internetStateLabel(context, summary.internetState)
    } else {
        internetStateLabel(context, summary.internetState)
    }
}

private fun internetStateLabel(context: Context, state: String): String = when (state) {
    "up" -> context.getString(R.string.internet_panel_state_up)
    "degraded" -> context.getString(R.string.internet_panel_state_degraded)
    "down" -> context.getString(R.string.internet_panel_state_down)
    else -> context.getString(R.string.widget_internet_none)
}

/** Misma frase que [com.baxner.vigia.ui.components.agoLabel], fuera de Compose UI (Glance no usa `stringResource`). */
private fun agoText(context: Context, updatedAtEpochMs: Long?): String {
    if (updatedAtEpochMs == null) return context.getString(R.string.widget_never_updated)
    return when (val relative = relativeTimeFrom(Instant.ofEpochMilli(updatedAtEpochMs))) {
        RelativeTime.JustNow -> context.getString(R.string.time_ago_moment)
        is RelativeTime.Seconds -> context.getString(R.string.panel_status_relative, context.getString(R.string.panel_time_seconds, relative.value.toInt()))
        is RelativeTime.Minutes -> context.getString(R.string.panel_status_relative, context.getString(R.string.panel_time_minutes, relative.value.toInt()))
        is RelativeTime.Hours -> context.getString(R.string.panel_status_relative, context.getString(R.string.panel_time_hours, relative.value.toInt()))
        is RelativeTime.Days -> context.getString(R.string.panel_status_relative, context.getString(R.string.panel_time_days, relative.value.toInt()))
    }
}
