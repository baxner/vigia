package com.baxner.vigia.ui.radar

import kotlin.math.abs
import android.content.Context
import android.media.AudioManager
import android.media.ToneGenerator
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.TrendingDown
import androidx.compose.material.icons.automirrored.filled.TrendingFlat
import androidx.compose.material.icons.automirrored.filled.TrendingUp
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import com.baxner.vigia.R
import com.baxner.vigia.data.radar.BearingConfidence
import com.baxner.vigia.data.radar.BearingEstimate
import com.baxner.vigia.data.radar.BearingEstimator
import com.baxner.vigia.data.radar.DistanceRange
import com.baxner.vigia.data.radar.RadarModule
import com.baxner.vigia.data.radar.SignalSource
import com.baxner.vigia.data.radar.SignalSourceState
import com.baxner.vigia.data.radar.HeadingSource
import com.baxner.vigia.data.radar.SignalTarget
import com.baxner.vigia.data.radar.Trend
import com.baxner.vigia.ui.components.KeepScreenOn
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.StatusChip
import com.baxner.vigia.ui.permissions.rememberBluetoothScanPermissionState
import com.baxner.vigia.ui.theme.VigiaColors
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import java.util.Locale
import kotlin.math.cos
import kotlin.math.log10
import kotlin.math.min
import kotlin.math.roundToInt
import kotlin.math.sin

/**
 * Envoltura con estado de "Buscar señal" (fase 6, ver docs/plan.md): crea
 * [HuntViewModel] con las fuentes reales de [RadarModule] (sustituibles en
 * pruebas/capturas por fakes vía los parámetros [signalSource]/
 * [headingSource]) y gobierna [HuntViewModel.start]/[HuntViewModel.stop] con
 * [repeatOnLifecycle]: solo escucha radio y sensores mientras la pantalla
 * está visible ("Fuentes y sensores solo con la pantalla visible").
 */
@Composable
fun HuntScreen(
    target: SignalTarget,
    onBack: () -> Unit,
    onRequestLocationPermission: () -> Unit,
    modifier: Modifier = Modifier,
    onRequestBluetoothPermission: (() -> Unit)? = null,
    signalSource: SignalSource? = null,
    headingSource: HeadingSource? = null,
) {
    val context = LocalContext.current
    val effectiveSignalSource = signalSource ?: remember { RadarModule.signalSource(context) }
    val effectiveHeadingSource = headingSource ?: remember { RadarModule.headingSource(context) }
    val huntViewModel = remember(effectiveSignalSource, effectiveHeadingSource) {
        HuntViewModel(effectiveSignalSource, effectiveHeadingSource)
    }
    DisposableEffect(huntViewModel) {
        onDispose { huntViewModel.onCleared() }
    }
    val lifecycleOwner = LocalLifecycleOwner.current

    LaunchedEffect(target, huntViewModel, lifecycleOwner) {
        lifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
            huntViewModel.start(target)
            try {
                awaitCancellation()
            } finally {
                huntViewModel.stop()
            }
        }
    }

    val uiState by huntViewModel.uiState.collectAsStateWithLifecycle()
    KeepScreenOn()
    GeigerEffect(uiState.geigerIntervalMillis, uiState.soundEnabled, uiState.vibrationEnabled)

    val isBle = target is SignalTarget.Ble
    val (_, defaultRequestBluetoothPermission) = rememberBluetoothScanPermissionState()
    val effectiveBlePermissionRequest = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
        onRequestBluetoothPermission ?: defaultRequestBluetoothPermission
    } else {
        onRequestLocationPermission
    }
    HuntScreenContent(
        uiState = uiState,
        onBack = onBack,
        onToggleSound = huntViewModel::toggleSound,
        onToggleVibration = huntViewModel::toggleVibration,
        onStartGuidedTurn = huntViewModel::startGuidedTurn,
        onRequestPermission = if (isBle) {
            effectiveBlePermissionRequest
        } else {
            onRequestLocationPermission
        },
        modifier = modifier,
    )
}

/** Versión sin estado (capturas, tests): ver [HuntScreen] para la envoltura real. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HuntScreenContent(
    uiState: HuntUiState,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    onToggleSound: () -> Unit = {},
    onToggleVibration: () -> Unit = {},
    onStartGuidedTurn: () -> Unit = {},
    onRequestPermission: () -> Unit = {},
) {
    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.hunt_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = VigiaColors.Background, titleContentColor = VigiaColors.TextPrimary),
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item(key = "header") { HuntHeader(uiState.target, uiState.rttMeters) }

            if (uiState.sourceState != SignalSourceState.OK) {
                item(key = "warning") {
                    HuntWarningCard(
                        state = uiState.sourceState,
                        isBle = uiState.target is SignalTarget.Ble,
                        onRequestPermission = onRequestPermission,
                    )
                }
            }

            item(key = "detector") {
                DetectorCard(uiState = uiState, onToggleSound = onToggleSound, onToggleVibration = onToggleVibration)
            }

            item(key = "radar") {
                RadarCard(uiState = uiState, onStartGuidedTurn = onStartGuidedTurn)
            }
        }
    }
}

@Composable
private fun HuntHeader(target: SignalTarget?, rttMeters: Double?) {
    SectionCard(title = "") {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(target?.label.orEmpty(), style = MaterialTheme.typography.titleLarge, color = VigiaColors.TextPrimary)
                Text(huntTypeLabel(target), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextSecondary)
            }
            if (rttMeters != null) {
                StatusChip(text = stringResource(R.string.hunt_rtt_chip))
            }
        }
    }
}

@Composable
private fun huntTypeLabel(target: SignalTarget?): String = when (target) {
    is SignalTarget.WifiAp -> stringResource(R.string.hunt_type_wifi)
    is SignalTarget.Ble -> stringResource(R.string.hunt_type_bluetooth)
    null -> ""
}

@Composable
private fun HuntWarningCard(state: SignalSourceState, isBle: Boolean, onRequestPermission: () -> Unit) {
    val message = when (state) {
        SignalSourceState.SLOW_THROTTLED -> stringResource(R.string.hunt_warning_slow_throttled)
        SignalSourceState.NO_PERMISSION -> stringResource(if (isBle) R.string.hunt_warning_no_permission_bluetooth else R.string.hunt_warning_no_permission_wifi)
        SignalSourceState.RADIO_OFF -> stringResource(if (isBle) R.string.hunt_warning_radio_off_bluetooth else R.string.hunt_warning_radio_off_wifi)
        SignalSourceState.NOT_FOUND -> stringResource(R.string.hunt_warning_not_found)
        SignalSourceState.OK -> ""
    }
    SectionCard(title = stringResource(R.string.panel_attention_title)) {
        Text(message, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
        if (state == SignalSourceState.NO_PERMISSION) {
            Button(
                onClick = onRequestPermission,
                colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background),
            ) { Text(stringResource(R.string.permissions_grant)) }
        }
    }
}

@Composable
private fun DetectorCard(uiState: HuntUiState, onToggleSound: () -> Unit, onToggleVibration: () -> Unit) {
    SectionCard(title = stringResource(R.string.hunt_detector_title)) {
        val rssi = uiState.smoothedRssi
        Text(
            text = rssi?.let { "${it.roundToInt()} dBm" } ?: "—",
            style = MaterialTheme.typography.displaySmall,
            color = VigiaColors.TextPrimary,
        )
        SignalLevelBar(rssiDbm = rssi?.roundToInt() ?: -100)

        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Icon(trendIcon(uiState.trend), contentDescription = null, tint = trendColor(uiState.trend))
            Text(trendLabel(uiState.trend), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
        }

        Text(distanceLabel(uiState), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)

        Row(
            modifier = Modifier.fillMaxWidth().toggleable(value = uiState.soundEnabled, onValueChange = { onToggleSound() }, role = Role.Switch),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(stringResource(R.string.hunt_sound_label), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
            Switch(checked = uiState.soundEnabled, onCheckedChange = null)
        }
        Row(
            modifier = Modifier.fillMaxWidth().toggleable(value = uiState.vibrationEnabled, onValueChange = { onToggleVibration() }, role = Role.Switch),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(stringResource(R.string.hunt_vibration_label), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
            Switch(checked = uiState.vibrationEnabled, onCheckedChange = null)
        }
    }
}

@Composable
private fun SignalLevelBar(rssiDbm: Int, modifier: Modifier = Modifier) {
    val fraction = ((rssiDbm + 90).coerceIn(0, 60) / 60f).coerceIn(0.04f, 1f)
    Box(
        modifier = modifier.fillMaxWidth().height(8.dp).background(VigiaColors.Outline, RoundedCornerShape(4.dp)),
    ) {
        Box(
            modifier = Modifier
                .fillMaxWidth(fraction)
                .height(8.dp)
                .background(
                    Brush.horizontalGradient(listOf(VigiaColors.Risk, VigiaColors.Amber, VigiaColors.RadarCyan)),
                    RoundedCornerShape(4.dp),
                ),
        )
    }
}

private fun trendIcon(trend: Trend): ImageVector = when (trend) {
    Trend.CLOSER -> Icons.AutoMirrored.Filled.TrendingUp
    Trend.FARTHER -> Icons.AutoMirrored.Filled.TrendingDown
    Trend.STEADY -> Icons.AutoMirrored.Filled.TrendingFlat
}

private fun trendColor(trend: Trend) = when (trend) {
    Trend.CLOSER -> VigiaColors.RadarCyan
    Trend.FARTHER -> VigiaColors.Amber
    Trend.STEADY -> VigiaColors.TextSecondary
}

@Composable
private fun trendLabel(trend: Trend): String = when (trend) {
    Trend.CLOSER -> stringResource(R.string.hunt_trend_closer)
    Trend.FARTHER -> stringResource(R.string.hunt_trend_farther)
    Trend.STEADY -> stringResource(R.string.hunt_trend_steady)
}

/** "≈ 2–5 m" por señal, o "1.8 m (medida)" cuando hay Wi-Fi RTT (más preciso, ver el encargo). */
@Composable
private fun distanceLabel(uiState: HuntUiState): String {
    uiState.rttMeters?.let { return stringResource(R.string.hunt_distance_measured, formatMetersPlain(it)) }
    return uiState.distance?.let { formatDistanceRangeLabel(it) } ?: "—"
}

internal fun formatMetersPlain(meters: Double): String = String.format(Locale.US, "%.1f", meters)

internal fun formatDistanceRangeLabel(range: DistanceRange): String {
    fun fmt(m: Double): String {
        val roundedTenths = (m * 10).roundToInt() / 10.0
        return if (roundedTenths < 1.0) String.format(Locale.US, "%.1f", roundedTenths) else roundedTenths.roundToInt().toString()
    }
    val lo = minOf(range.minMeters, range.maxMeters)
    val hi = maxOf(range.minMeters, range.maxMeters)
    // Muy cerca, un rango tipo "0.3–0.6 m" aparenta una precisión que la señal no da.
    if (hi < 1.0) return "< 1 m"
    return "≈ ${fmt(lo)}–${fmt(hi)} m"
}

@Composable
private fun RadarCard(uiState: HuntUiState, onStartGuidedTurn: () -> Unit) {
    val minPerSector = if (uiState.target is SignalTarget.Ble) 5 else 2
    // Descripción resumida del radar para TalkBack (ver AGENTS.md §
    // accesibilidad): el rumbo y la confianza, no la posición punto por
    // punto de la cuadrícula/sectores.
    val radarDescription = uiState.bearing?.let { bearing ->
        uiState.headingDeg?.let { heading ->
            "${turnHint(bearing.bearingDeg, heading.toDouble())}. ${bearingConfidenceLabel(bearing.confidence)}"
        }
    } ?: stringResource(R.string.hunt_radar_title)
    SectionCard(title = stringResource(R.string.hunt_radar_title)) {
        CompassRadar(
            headingDeg = uiState.headingDeg,
            bearing = uiState.bearing,
            distance = uiState.distance,
            sectorCounts = uiState.sectorCounts,
            minPerSector = minPerSector,
            contentDescription = radarDescription,
            modifier = Modifier.fillMaxWidth(),
        )

        if (uiState.guidedTurnActive) {
            Text(
                stringResource(R.string.hunt_guided_turn_instruction),
                style = MaterialTheme.typography.bodyMedium,
                color = VigiaColors.TextSecondary,
            )
        }
        uiState.bearing?.let { bearing ->
            if (bearing.confidence != BearingConfidence.INCONCLUSIVE) {
                uiState.headingDeg?.let { heading ->
                    Text(
                        turnHint(bearing.bearingDeg, heading.toDouble()),
                        style = MaterialTheme.typography.titleMedium,
                        color = VigiaColors.RadarCyan,
                    )
                }
            }
            Text(bearingConfidenceLabel(bearing.confidence), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
        }

        OutlinedButton(onClick = onStartGuidedTurn) {
            Text(stringResource(R.string.hunt_guided_turn_button))
        }
    }
}

/** Hacia dónde girar, en palabras: el rumbo estimado relativo a hacia dónde apunta el teléfono. */
@Composable
private fun turnHint(bearingDeg: Double, headingDeg: Double): String {
    val relative = relativeTurnDegrees(bearingDeg, headingDeg)
    val degrees = abs(relative).roundToInt()
    return when {
        degrees <= 15 -> stringResource(R.string.hunt_turn_ahead)
        degrees >= 165 -> stringResource(R.string.hunt_turn_behind)
        relative > 0 -> stringResource(R.string.hunt_turn_right, degrees)
        else -> stringResource(R.string.hunt_turn_left, degrees)
    }
}

/** Ángulo de giro de −180 a 180: positivo, a la derecha (sentido horario). */
internal fun relativeTurnDegrees(bearingDeg: Double, headingDeg: Double): Double {
    val d = ((bearingDeg - headingDeg) % 360 + 360) % 360
    return if (d > 180) d - 360 else d
}

@Composable
private fun bearingConfidenceLabel(confidence: BearingConfidence): String = when (confidence) {
    BearingConfidence.HIGH -> stringResource(R.string.hunt_bearing_confidence_high)
    BearingConfidence.MEDIUM -> stringResource(R.string.hunt_bearing_confidence_medium)
    BearingConfidence.INCONCLUSIVE -> stringResource(R.string.hunt_bearing_confidence_inconclusive)
}

/**
 * Radar con brújula: el rumbo del teléfono siempre arriba (modo "heading-up",
 * como un GPS de coche). Todo lo que es absoluto (marca del norte, objetivo,
 * anillo de sectores) se dibuja en su ángulo de brújula real y se envuelve en
 * un único [rotate] por `-headingDeg`; así cuando el teléfono gira, esos
 * elementos giran al revés en pantalla y lo que apunta "arriba" sigue siendo
 * hacia donde mira el teléfono. La punta fija (el teléfono) se dibuja fuera
 * de ese bloque, siempre derecho hacia arriba.
 */
@Composable
private fun CompassRadar(
    headingDeg: Float?,
    bearing: BearingEstimate?,
    distance: DistanceRange?,
    sectorCounts: List<Int>,
    minPerSector: Int,
    contentDescription: String,
    modifier: Modifier = Modifier,
) {
    val textMeasurer = rememberTextMeasurer()
    Canvas(modifier = modifier.aspectRatio(1f).semantics { this.contentDescription = contentDescription }) {
        val maxRadius = min(size.width, size.height) / 2f * 0.84f
        val center = Offset(size.width / 2f, size.height / 2f)
        val heading = headingDeg ?: 0f

        // Anillos concéntricos (fijos, no rotan: son solo la escala de distancia).
        listOf(0.3f, 0.55f, 0.8f, 1f).forEach { f ->
            drawCircle(
                color = VigiaColors.RadarCyan.copy(alpha = 0.14f),
                radius = maxRadius * f,
                center = center,
                style = Stroke(width = 1.dp.toPx()),
            )
        }

        rotate(degrees = -heading, pivot = center) {
            // Anillo de 8 sectores de la vuelta guiada, en el borde exterior.
            for (sector in 0 until BearingEstimator.Sectors) {
                val count = sectorCounts.getOrElse(sector) { 0 }
                val fraction = (count.toFloat() / minPerSector).coerceIn(0f, 1f)
                drawArc(
                    color = VigiaColors.RadarCyan.copy(alpha = 0.16f + 0.55f * fraction),
                    startAngle = sector * 45f - 90f + 2f,
                    sweepAngle = 41f,
                    useCenter = false,
                    topLeft = Offset(center.x - maxRadius, center.y - maxRadius),
                    size = androidx.compose.ui.geometry.Size(maxRadius * 2f, maxRadius * 2f),
                    style = Stroke(width = 6.dp.toPx()),
                )
            }

            // Marca del norte (bearing absoluto 0°).
            val northRad = Math.toRadians(-90.0)
            val northPos = center + Offset(cos(northRad).toFloat(), sin(northRad).toFloat()) * (maxRadius * 1f)
            drawCircle(color = VigiaColors.TextSecondary, radius = 3.dp.toPx(), center = northPos)
            val label = textMeasurer.measure("N")
            drawText(label, topLeft = northPos - Offset(label.size.width / 2f, label.size.height / 2f + 10.dp.toPx()), color = VigiaColors.TextSecondary)

            // El objetivo, solo si ya hay un rumbo estimado (la vuelta guiada se completó).
            if (bearing != null) {
                val angleRad = Math.toRadians(bearing.bearingDeg - 90.0)
                val radiusFraction = distanceToRingFraction(distance?.typicalMeters)
                val pos = center + Offset(cos(angleRad).toFloat(), sin(angleRad).toFloat()) * (maxRadius * radiusFraction)
                drawCircle(
                    brush = Brush.radialGradient(
                        colors = listOf(VigiaColors.RadarCyan.copy(alpha = 0.4f), androidx.compose.ui.graphics.Color.Transparent),
                        center = pos,
                        radius = 18.dp.toPx(),
                    ),
                    radius = 18.dp.toPx(),
                    center = pos,
                )
                drawCircle(color = VigiaColors.RadarCyan, radius = 7.dp.toPx(), center = pos)
            }
        }

        // Puntero fijo del teléfono: siempre derecho hacia arriba.
        val tip = center + Offset(0f, -maxRadius * 1.08f)
        val left = center + Offset(-7.dp.toPx(), -maxRadius * 0.92f)
        val right = center + Offset(7.dp.toPx(), -maxRadius * 0.92f)
        drawPath(
            path = androidx.compose.ui.graphics.Path().apply {
                moveTo(tip.x, tip.y)
                lineTo(left.x, left.y)
                lineTo(right.x, right.y)
                close()
            },
            color = VigiaColors.TextPrimary,
        )
        drawCircle(color = VigiaColors.RadarCyan.copy(alpha = 0.5f), radius = 4.dp.toPx(), center = center)
    }
}

/** Mapea la distancia típica (0.3–60 m, el rango de [com.baxner.vigia.data.radar.DistanceModel]) a una fracción del radio, en escala logarítmica. */
private fun distanceToRingFraction(typicalMeters: Double?): Float {
    if (typicalMeters == null) return 0.55f
    val minLog = log10(0.3)
    val maxLog = log10(60.0)
    val t = ((log10(typicalMeters.coerceIn(0.3, 60.0)) - minLog) / (maxLog - minLog)).toFloat()
    return (0.12f + t * 0.85f).coerceIn(0.12f, 0.97f)
}

/**
 * Tono tipo Geiger y vibración corta, cada [intervalMillis] (ver
 * [HuntUiState.geigerIntervalMillis]): se aceleran solos al cambiar el
 * intervalo porque [LaunchedEffect] reinicia el bucle. Cada tic va en
 * `runCatching` a propósito: `ToneGenerator`/`Vibrator` son servicios del
 * sistema que no deben tirar la pantalla abajo si fallan en algún teléfono.
 */
@Composable
private fun GeigerEffect(intervalMillis: Long?, soundEnabled: Boolean, vibrationEnabled: Boolean) {
    val context = LocalContext.current
    val toneGenerator = remember { runCatching { ToneGenerator(AudioManager.STREAM_SYSTEM, 70) }.getOrNull() }
    DisposableEffect(Unit) { onDispose { runCatching { toneGenerator?.release() } } }
    val vibrator = remember { vibratorFor(context) }

    LaunchedEffect(intervalMillis, soundEnabled, vibrationEnabled) {
        val interval = intervalMillis ?: return@LaunchedEffect
        while (isActive) {
            if (soundEnabled) runCatching { toneGenerator?.startTone(ToneGenerator.TONE_PROP_BEEP, 35) }
            if (vibrationEnabled) runCatching { vibrator?.vibrate(VibrationEffect.createOneShot(25, VibrationEffect.DEFAULT_AMPLITUDE)) }
            delay(interval)
        }
    }
}

private fun vibratorFor(context: Context): Vibrator? = runCatching {
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
        (context.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as? VibratorManager)?.defaultVibrator
    } else {
        @Suppress("DEPRECATION")
        context.getSystemService(Context.VIBRATOR_SERVICE) as? Vibrator
    }
}.getOrNull()
