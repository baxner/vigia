package com.baxner.vigia.ui.internet

import android.text.format.DateFormat
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.Remove
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.InternetSeriesPointDto
import com.baxner.vigia.data.agent.InternetSeriesRange
import com.baxner.vigia.data.agent.InternetState
import com.baxner.vigia.data.agent.InternetStatusDto
import com.baxner.vigia.data.agent.Ipv6State
import com.baxner.vigia.data.agent.OutageDto
import com.baxner.vigia.data.agent.SpeedTestDto
import com.baxner.vigia.data.agent.blameOrUnknown
import com.baxner.vigia.data.agent.ipv6OrUnknown
import com.baxner.vigia.data.agent.stateOrUnknown
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.GlassSurface
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.agoLabel
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import com.baxner.vigia.util.RelativeTime
import com.baxner.vigia.util.formatDateTime
import com.baxner.vigia.util.relativeTimeFrom
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneOffset
import java.time.format.DateTimeParseException
import kotlin.math.roundToInt

/**
 * Envoltura con estado de la pantalla Internet (ver el encargo, "Qué
 * construir", punto 2): conecta [InternetViewModel] con la versión sin
 * estado [InternetScreenContent] (capturas, tests). Se llega acá desde la
 * fila "Internet" de la tarjeta "Vigilancia 24/7" del Panel (ver
 * `ui/agent/AgentCard.kt` y `ui/navigation/VigiaNavHost.kt`).
 */
@Composable
fun InternetScreen(onBack: () -> Unit, modifier: Modifier = Modifier) {
    val viewModel: InternetViewModel = viewModel()
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) { viewModel.onOpen() }

    InternetScreenContent(
        uiState = uiState,
        onBack = onBack,
        onSetRange = viewModel::setSeriesRange,
        onRunSpeedTestNow = viewModel::runSpeedTestNow,
        onSetSpeedTestsPerDay = viewModel::setSpeedTestsPerDay,
        modifier = modifier,
    )
}

/** Versión sin estado (capturas, tests): ver [InternetScreen] para la envoltura real. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InternetScreenContent(
    uiState: InternetUiState,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    onSetRange: (InternetSeriesRange) -> Unit = {},
    onRunSpeedTestNow: () -> Unit = {},
    onSetSpeedTestsPerDay: (Int) -> Unit = {},
    now: Instant = Instant.now(),
) {
    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.internet_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = VigiaColors.Background, titleContentColor = VigiaColors.TextPrimary),
            )
        },
    ) { padding ->
        when {
            !uiState.linked -> EmptyState(
                title = stringResource(R.string.internet_empty_not_linked_title),
                subtitle = stringResource(R.string.internet_empty_not_linked_subtitle),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
            !uiState.monitoring -> EmptyState(
                title = stringResource(R.string.internet_empty_not_monitoring_title),
                subtitle = stringResource(R.string.internet_empty_not_monitoring_subtitle),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
            uiState.status == null && uiState.loading -> Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) {
                CircularProgressIndicator(color = VigiaColors.RadarCyan)
            }
            uiState.status == null -> EmptyState(
                title = stringResource(R.string.internet_empty_not_linked_title),
                subtitle = stringResource(R.string.internet_empty_not_linked_subtitle),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
            else -> InternetBody(
                status = uiState.status,
                padding = padding,
                seriesRange = uiState.seriesRange,
                series = uiState.series,
                onSetRange = onSetRange,
                speedTestRunning = uiState.speedTestRunning,
                speedTestMessage = uiState.speedTestMessage,
                speedTestsPerDay = uiState.settings.speedTestsPerDay,
                onRunSpeedTestNow = onRunSpeedTestNow,
                onSetSpeedTestsPerDay = onSetSpeedTestsPerDay,
                now = now,
            )
        }
    }
}

@Composable
private fun InternetBody(
    status: InternetStatusDto,
    padding: PaddingValues,
    seriesRange: InternetSeriesRange,
    series: List<InternetSeriesPointDto>,
    onSetRange: (InternetSeriesRange) -> Unit,
    speedTestRunning: Boolean,
    speedTestMessage: SpeedTestMessage?,
    speedTestsPerDay: Int,
    onRunSpeedTestNow: () -> Unit,
    onSetSpeedTestsPerDay: (Int) -> Unit,
    now: Instant,
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize().padding(padding),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item(key = "header") { InternetHeader(status, now) }

        item(key = "graph") {
            LatencyGraphCard(
                series = series,
                outages = status.outages,
                range = seriesRange,
                onSetRange = onSetRange,
                now = now,
            )
        }

        item(key = "outages") { OutagesCard(status.outages) }

        item(key = "speed") {
            SpeedCard(
                speedTests = status.speedTests,
                speedTestRunning = speedTestRunning,
                speedTestMessage = speedTestMessage,
                speedTestsPerDay = speedTestsPerDay,
                onRunSpeedTestNow = onRunSpeedTestNow,
                onSetSpeedTestsPerDay = onSetSpeedTestsPerDay,
            )
        }

        if (status.ipv6OrUnknown() == Ipv6State.BROKEN) {
            item(key = "ipv6") {
                SectionCard(title = stringResource(R.string.internet_ipv6_title)) {
                    Text(stringResource(R.string.internet_ipv6_broken_note), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.Amber)
                }
            }
        }
    }
}

@Composable
private fun InternetHeader(status: InternetStatusDto, now: Instant) {
    val state = status.stateOrUnknown()
    val (titleRes, color) = when (state) {
        InternetState.UP -> R.string.internet_state_up_title to VigiaColors.Ok
        InternetState.DEGRADED -> R.string.internet_state_degraded_title to VigiaColors.Amber
        InternetState.DOWN -> R.string.internet_state_down_title to VigiaColors.Risk
        InternetState.UNKNOWN -> R.string.internet_state_unknown_title to VigiaColors.TextTertiary
    }
    GlassSurface(modifier = Modifier.fillMaxWidth(), glow = state == InternetState.DOWN) {
        Column(modifier = Modifier.fillMaxWidth().padding(18.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(stringResource(titleRes), style = MaterialTheme.typography.headlineSmall, color = color)
            status.since?.let { since ->
                relativeTimeFromNow(since, now)?.let { relative ->
                    Text(stringResource(R.string.internet_since_label, agoLabel(relative)), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
                }
            }
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                HopColumn(stringResource(R.string.internet_hop_router), status.now.routerMs, modifier = Modifier.weight(1f))
                HopColumn(stringResource(R.string.internet_hop_modem), status.now.modemMs, modifier = Modifier.weight(1f))
                HopColumn(stringResource(R.string.internet_hop_internet), status.now.internetMs, highlight = true, modifier = Modifier.weight(1f))
                HopColumn(stringResource(R.string.internet_hop_dns), status.now.dnsMs, modifier = Modifier.weight(1f))
            }
            if (status.now.lossPct > 0) {
                Text(
                    stringResource(R.string.internet_loss_label, status.now.lossPct.roundToInt()),
                    style = MaterialTheme.typography.bodySmall,
                    color = VigiaColors.Amber,
                )
            }
        }
    }
}

@Composable
private fun HopColumn(label: String, ms: Double?, modifier: Modifier = Modifier, highlight: Boolean = false) {
    Column(modifier = modifier, horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
            ms?.let { stringResource(R.string.internet_ms_format, it.roundToInt().toString()) } ?: stringResource(R.string.internet_no_data),
            style = if (highlight) VigiaMono.bodyMedium else VigiaMono.body,
            color = if (ms != null) VigiaColors.TextPrimary else VigiaColors.TextTertiary,
            textAlign = TextAlign.Center,
        )
        Text(label, style = MaterialTheme.typography.labelSmall, color = VigiaColors.TextSecondary, textAlign = TextAlign.Center)
    }
}

@Composable
private fun LatencyGraphCard(
    series: List<InternetSeriesPointDto>,
    outages: List<OutageDto>,
    range: InternetSeriesRange,
    onSetRange: (InternetSeriesRange) -> Unit,
    now: Instant,
) {
    SectionCard(title = stringResource(R.string.internet_graph_title)) {
        RangeSelector(selected = range, onSelect = onSetRange)
        if (series.isEmpty()) {
            Text(stringResource(R.string.internet_graph_empty), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        } else {
            LatencyChart(series = series, outages = outages, seriesRange = range, now = now, modifier = Modifier.padding(top = 4.dp))
        }
    }
}

@Composable
private fun RangeSelector(selected: InternetSeriesRange, onSelect: (InternetSeriesRange) -> Unit) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        RangeChip(stringResource(R.string.internet_range_24h), selected == InternetSeriesRange.HOURS_24) { onSelect(InternetSeriesRange.HOURS_24) }
        RangeChip(stringResource(R.string.internet_range_7d), selected == InternetSeriesRange.DAYS_7) { onSelect(InternetSeriesRange.DAYS_7) }
    }
}

@Composable
private fun RangeChip(label: String, selected: Boolean, onClick: () -> Unit) {
    Surface(
        shape = RoundedCornerShape(50),
        color = if (selected) VigiaColors.RadarCyan.copy(alpha = 0.18f) else VigiaColors.SurfaceElevated,
        border = BorderStroke(1.dp, if (selected) VigiaColors.RadarCyan else VigiaColors.Outline),
        // selectable (no clickable): declara el estado "seleccionado" en la
        // semántica (ver AGENTS.md § accesibilidad) — TalkBack anuncia
        // "24 h, seleccionado" en vez de solo el texto visible. Sin
        // minimumInteractiveComponentSize() a propósito: agrandaría la fila
        // completa y cambiaría la captura con letra normal.
        modifier = Modifier
            .minimumInteractiveComponentSize()
            .selectable(selected = selected, onClick = onClick, role = Role.RadioButton),
    ) {
        Text(
            label,
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp),
            color = if (selected) VigiaColors.RadarCyan else VigiaColors.TextSecondary,
            style = MaterialTheme.typography.labelMedium,
        )
    }
}

/**
 * La gráfica "a mano" que pide el encargo (ver "Qué construir", punto 2:
 * "dibujada a mano con Canvas, sin librería nueva"): la línea del tramo
 * internet, con huecos donde [InternetSeriesPointDto.internetMs] es `null`
 * (ver [LatencyGraphScaling.segments]) y marcas rojas en los puntos que
 * caen dentro de una caída (ver [LatencyGraphScaling.outageMarkerIndices]).
 */
@Composable
private fun LatencyChart(
    series: List<InternetSeriesPointDto>,
    outages: List<OutageDto>,
    seriesRange: InternetSeriesRange,
    now: Instant,
    modifier: Modifier = Modifier,
) {
    val values = remember(series) { series.map { it.internetMs } }
    val timestamps = remember(series) { series.map { it.at } }
    val valueRange = remember(values) { LatencyGraphScaling.rangeOf(values) }
    val segments = remember(values) { LatencyGraphScaling.segments(values) }
    val markers = remember(timestamps, outages, now) { LatencyGraphScaling.outageMarkerIndices(timestamps, outages, now) }
    val lineColor = VigiaColors.RadarCyan
    val markerColor = VigiaColors.Risk

    // Descripción resumida con el rango que se está viendo (24 h o 7 días,
    // ver [InternetSeriesRange] y el selector arriba): antes decía "última
    // hora" fijo sin importar qué rango estaba seleccionado.
    val descriptionRes = if (seriesRange == InternetSeriesRange.DAYS_7) {
        R.string.internet_graph_description_7d
    } else {
        R.string.internet_graph_description_24h
    }
    val graphDescription = stringResource(descriptionRes, valueRange.min.roundToInt().toString(), valueRange.max.roundToInt().toString())
    androidx.compose.foundation.layout.Box(
        modifier = modifier
            .fillMaxWidth()
            .semantics { contentDescription = graphDescription }
    ) {
    Canvas(modifier = Modifier.fillMaxWidth().height(140.dp)) {
        if (values.size < 2) return@Canvas
        val stepX = size.width / (values.size - 1)
        fun pointFor(index: Int, value: Double): Offset {
            val x = stepX * index
            val y = size.height * (1f - valueRange.normalize(value))
            return Offset(x, y)
        }
        segments.forEach { segment ->
            if (segment.size == 1) {
                val index = segment.first()
                val value = values[index] ?: return@forEach
                drawCircle(color = lineColor, radius = 2.5.dp.toPx(), center = pointFor(index, value))
                return@forEach
            }
            val path = Path()
            segment.forEachIndexed { i, index ->
                val value = values[index] ?: return@forEachIndexed
                val point = pointFor(index, value)
                if (i == 0) path.moveTo(point.x, point.y) else path.lineTo(point.x, point.y)
            }
            drawPath(path = path, color = lineColor, style = Stroke(width = 2.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round))
        }
        markers.forEach { index ->
            val value = values[index] ?: return@forEach
            drawCircle(color = markerColor, radius = 3.5.dp.toPx(), center = pointFor(index, value))
        }
    }
    if (values.any { it != null }) {
        Text(
            "${valueRange.max.roundToInt()} ms",
            style = VigiaMono.labelSmall,
            color = VigiaColors.TextTertiary,
            modifier = Modifier.align(Alignment.TopEnd),
        )
        Text(
            "${valueRange.min.roundToInt()} ms",
            style = VigiaMono.labelSmall,
            color = VigiaColors.TextTertiary,
            modifier = Modifier.align(Alignment.BottomEnd),
        )
    }
    }
}

@Composable
private fun OutagesCard(outages: List<OutageDto>) {
    SectionCard(title = stringResource(R.string.internet_outages_title)) {
        if (outages.isEmpty()) {
            Text(stringResource(R.string.internet_outages_empty), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        } else {
            outages.take(MaxOutageRows).forEach { outage -> OutageRow(outage) }
            val remaining = outages.size - MaxOutageRows
            if (remaining > 0) {
                Text(stringResource(R.string.common_and_more, remaining), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
            }
        }
    }
}

private const val MaxOutageRows = 10

@Composable
private fun OutageRow(outage: OutageDto) {
    val ongoing = outage.end == null
    val duration = localizedOutageDuration(outage.durationSec, ongoing)
    val blame = outage.blameOrUnknown().localizedLabel()
    val is24Hour = DateFormat.is24HourFormat(LocalContext.current)
    val dateLabel = remember(outage, is24Hour) {
        parseInstantOrNull(outage.start)?.let { formatDateTime(it, is24Hour) } ?: outage.start
    }
    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            if (ongoing) Icons.Default.ErrorOutline else Icons.Default.History,
            contentDescription = null,
            tint = if (ongoing) VigiaColors.Risk else VigiaColors.TextTertiary,
            modifier = Modifier.padding(top = 2.dp),
        )
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(dateLabel, style = VigiaMono.body, color = VigiaColors.TextPrimary)
            Text(
                "$duration · $blame",
                style = MaterialTheme.typography.bodySmall,
                color = if (ongoing) VigiaColors.Risk else VigiaColors.TextSecondary,
            )
        }
    }
}

@Composable
private fun SpeedCard(
    speedTests: List<SpeedTestDto>,
    speedTestRunning: Boolean,
    speedTestMessage: SpeedTestMessage?,
    speedTestsPerDay: Int,
    onRunSpeedTestNow: () -> Unit,
    onSetSpeedTestsPerDay: (Int) -> Unit,
) {
    SectionCard(title = stringResource(R.string.internet_speed_title)) {
        val latest = speedTests.firstOrNull()
        if (latest == null) {
            Text(stringResource(R.string.internet_speed_none), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        } else {
            SpeedSummary(latest)
        }

        if (speedTestRunning) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                CircularProgressIndicator(modifier = Modifier.height(16.dp), strokeWidth = 2.dp, color = VigiaColors.RadarCyan)
                Text(stringResource(R.string.internet_speed_testing), color = VigiaColors.TextSecondary)
            }
        } else {
            Button(
                onClick = onRunSpeedTestNow,
                colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background),
            ) {
                Text(stringResource(R.string.internet_speed_test_now))
            }
        }
        speedTestMessage?.let { message ->
            val text = when (message) {
                SpeedTestMessage.AlreadyRunning -> stringResource(R.string.internet_speed_test_already_running)
                is SpeedTestMessage.Failed -> stringResource(R.string.internet_speed_test_error, message.detail)
            }
            Text(text, style = MaterialTheme.typography.bodySmall, color = VigiaColors.Risk)
        }

        if (speedTests.size > 1) {
            Text(stringResource(R.string.internet_speed_history_title), style = MaterialTheme.typography.labelMedium, color = VigiaColors.TextSecondary)
            speedTests.drop(1).take(MaxSpeedHistoryRows).forEach { test -> SpeedHistoryRow(test) }
        }

        SpeedTestsPerDaySetting(speedTestsPerDay, onSetSpeedTestsPerDay)
    }
}

private const val MaxSpeedHistoryRows = 5

@Composable
private fun SpeedSummary(test: SpeedTestDto) {
    if (test.error != null) {
        Text(stringResource(R.string.internet_speed_test_error, test.error), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.Risk)
        return
    }
    // Columnas de ancho igual (weight), no SpaceBetween: con 4 textos de
    // ancho variable ("95.4 Mbps" vs "11 ms"), SpaceBetween los separa por
    // un hueco fijo que no alcanza para los más anchos — se pisaban con la
    // columna vecina (visto en la captura de revisión, "Qué construir").
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        SpeedColumn(stringResource(R.string.internet_speed_down), test.downMbps, modifier = Modifier.weight(1f))
        SpeedColumn(stringResource(R.string.internet_speed_up), test.upMbps, modifier = Modifier.weight(1f))
        LatencyColumn(stringResource(R.string.internet_speed_latency_idle), test.latencyMs, modifier = Modifier.weight(1f))
        LatencyColumn(stringResource(R.string.internet_speed_latency_loaded), test.loadedLatencyMs, modifier = Modifier.weight(1f))
    }
}

@Composable
private fun SpeedColumn(label: String, mbps: Double?, modifier: Modifier = Modifier) {
    Column(modifier = modifier, horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
            mbps?.let { stringResource(R.string.internet_mbps_format, formatOneDecimal(it)) } ?: stringResource(R.string.internet_no_data),
            style = VigiaMono.bodyMedium,
            color = VigiaColors.TextPrimary,
            textAlign = TextAlign.Center,
        )
        Text(label, style = MaterialTheme.typography.labelSmall, color = VigiaColors.TextSecondary, textAlign = TextAlign.Center)
    }
}

@Composable
private fun LatencyColumn(label: String, ms: Double?, modifier: Modifier = Modifier) {
    Column(modifier = modifier, horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
            ms?.let { stringResource(R.string.internet_ms_format, it.roundToInt().toString()) } ?: stringResource(R.string.internet_no_data),
            style = VigiaMono.body,
            color = VigiaColors.TextPrimary,
            textAlign = TextAlign.Center,
        )
        Text(label, style = MaterialTheme.typography.labelSmall, color = VigiaColors.TextSecondary, textAlign = TextAlign.Center)
    }
}

@Composable
private fun SpeedHistoryRow(test: SpeedTestDto) {
    val is24Hour = DateFormat.is24HourFormat(LocalContext.current)
    val dateLabel = remember(test, is24Hour) {
        parseInstantOrNull(test.at)?.let { formatDateTime(it, is24Hour) } ?: test.at
    }
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(dateLabel, style = VigiaMono.labelSmall, color = VigiaColors.TextTertiary, modifier = Modifier.weight(1f))
        if (test.error != null) {
            Text(test.error, style = MaterialTheme.typography.labelSmall, color = VigiaColors.Risk)
        } else {
            Text(
                "${test.downMbps?.let(::formatOneDecimal) ?: "—"} / ${test.upMbps?.let(::formatOneDecimal) ?: "—"} Mbps",
                style = VigiaMono.labelSmall,
                color = VigiaColors.TextSecondary,
            )
        }
    }
}

@Composable
private fun SpeedTestsPerDaySetting(value: Int, onChange: (Int) -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(stringResource(R.string.internet_speed_tests_per_day_label), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            IconButton(onClick = { if (value > 0) onChange(value - 1) }) {
                Icon(Icons.Default.Remove, contentDescription = stringResource(R.string.internet_speed_tests_decrease), tint = VigiaColors.RadarCyan)
            }
            Text("$value", style = VigiaMono.bodyMedium, color = VigiaColors.TextPrimary)
            IconButton(onClick = { if (value < 6) onChange(value + 1) }) {
                Icon(Icons.Default.Add, contentDescription = stringResource(R.string.internet_speed_tests_increase), tint = VigiaColors.RadarCyan)
            }
        }
    }
}

private fun formatOneDecimal(value: Double): String = "%.1f".format(value)

/** [relativeTimeFrom] con un reloj fijo (ver el encargo, "Pruebas": "Reloj fijo en las pruebas"), igual criterio que `SecurityScreen.kt`. */
private fun relativeTimeFromNow(rfc3339: String, now: Instant): RelativeTime? =
    relativeTimeFrom(rfc3339, OffsetDateTime.ofInstant(now, ZoneOffset.UTC))

private fun parseInstantOrNull(rfc3339: String): Instant? =
    try {
        OffsetDateTime.parse(rfc3339).toInstant()
    } catch (_: DateTimeParseException) {
        null
    }
