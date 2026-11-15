package com.baxner.vigia.ui.components

import android.provider.Settings
import androidx.compose.animation.core.animateIntAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.baxner.vigia.R
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono
import com.baxner.vigia.util.RelativeTime

/**
 * true si el usuario desactivó las animaciones del sistema (Opciones de
 * desarrollador → "Escala de animador de ventana" = 0, un ajuste que también
 * activan algunos modos de accesibilidad). El barrido del radar y otras
 * animaciones continuas lo respetan para no forzar movimiento a quien lo apagó.
 */
@Composable
fun rememberReduceMotion(): Boolean {
    val context = LocalContext.current
    return remember {
        runCatching {
            Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
        }.getOrDefault(false)
    }
}

/**
 * Mantiene la pantalla encendida mientras el composable que la llama esté en
 * pantalla ("Buscar señal", fase 6, ver docs/plan.md: "Pantalla encendida
 * mientras se busca"): sin esto, el teléfono se apaga a media vuelta guiada.
 * Se apaga sola al salir de la pantalla ([DisposableEffect.onDispose]).
 */
@Composable
fun KeepScreenOn() {
    val view = LocalView.current
    DisposableEffect(view) {
        view.keepScreenOn = true
        onDispose { view.keepScreenOn = false }
    }
}

/** Brillo sutil detrás de una superficie: un degradado radial cian muy tenue, un poco más grande que ella. Úsalo solo en lo importante. */
fun Modifier.vigiaGlow(color: Color = VigiaColors.RadarCyan, alpha: Float = 0.18f): Modifier = this.drawBehind {
    val radius = (maxOf(size.width, size.height) / 2f) * 1.2f
    val center = Offset(size.width / 2f, size.height / 2f)
    drawCircle(
        brush = Brush.radialGradient(colors = listOf(color.copy(alpha = alpha), Color.Transparent), center = center, radius = radius),
        radius = radius,
        center = center,
    )
}

/**
 * La superficie "vidrio" de Radar Nocturno: contorno de 1 dp, esquinas de
 * 20 dp y, en lo importante ([glow]), un brillo cian detrás. Es el bloque que
 * más se repite en toda la app.
 */
@Composable
fun GlassSurface(
    modifier: Modifier = Modifier,
    glow: Boolean = false,
    glowColor: Color = VigiaColors.RadarCyan,
    shape: Shape = RoundedCornerShape(20.dp),
    containerColor: Color = VigiaColors.Surface,
    content: @Composable () -> Unit,
) {
    Box(
        modifier = modifier
            .then(if (glow) Modifier.vigiaGlow(color = glowColor) else Modifier)
            .background(containerColor, shape)
            .border(1.dp, VigiaColors.Outline, shape),
    ) {
        content()
    }
}

/**
 * Tarjeta con título chico arriba y contenido libre abajo, en vidrio: el
 * bloque que más se repite en Panel/Detalle (red, puertos de riesgo,
 * resumen Wi-Fi...).
 */
@Composable
fun SectionCard(
    title: String,
    modifier: Modifier = Modifier,
    glow: Boolean = false,
    content: @Composable ColumnScope.() -> Unit,
) {
    GlassSurface(modifier = modifier.fillMaxWidth(), glow = glow) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(text = title, style = MaterialTheme.typography.titleMedium, color = VigiaColors.TextPrimary)
            content()
        }
    }
}

/**
 * Campo de texto con los colores de "Radar Nocturno": el que más se repite
 * en formularios (Editar, el buscador de tipo). Envuelve [OutlinedTextField]
 * con el mismo contorno/relleno que el resto de la app.
 */
@Composable
fun VigiaTextField(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    placeholder: String? = null,
    singleLine: Boolean = true,
    minLines: Int = 1,
    leadingIcon: (@Composable () -> Unit)? = null,
) {
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        modifier = modifier.fillMaxWidth(),
        placeholder = placeholder?.let { hint -> { Text(hint, color = VigiaColors.TextTertiary) } },
        leadingIcon = leadingIcon,
        singleLine = singleLine,
        minLines = minLines,
        shape = RoundedCornerShape(14.dp),
        colors = OutlinedTextFieldDefaults.colors(
            focusedBorderColor = VigiaColors.RadarCyan,
            unfocusedBorderColor = VigiaColors.Outline,
            focusedTextColor = VigiaColors.TextPrimary,
            unfocusedTextColor = VigiaColors.TextPrimary,
            cursorColor = VigiaColors.RadarCyan,
            focusedContainerColor = VigiaColors.Surface,
            unfocusedContainerColor = VigiaColors.Surface,
        ),
    )
}

/** Icono grande + texto, centrado: "sin red", "sin dispositivos", etc. */
@Composable
fun EmptyState(
    title: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
) {
    Column(
        modifier = modifier.fillMaxWidth().padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(
            text = title,
            style = MaterialTheme.typography.bodyLarge,
            color = VigiaColors.TextSecondary,
            textAlign = TextAlign.Center,
        )
        if (subtitle != null) {
            Text(
                text = subtitle,
                style = MaterialTheme.typography.bodyMedium,
                color = VigiaColors.TextTertiary,
                textAlign = TextAlign.Center,
            )
        }
    }
}

/** 4 barritas de señal, con [level] (0–4) resaltadas en cian. Ver [com.baxner.vigia.data.network.rssiToBars]. */
@Composable
fun SignalBars(
    level: Int,
    modifier: Modifier = Modifier,
    barCount: Int = 4,
    activeColor: Color = VigiaColors.RadarCyan,
    inactiveColor: Color = VigiaColors.Outline,
) {
    Row(
        modifier = modifier,
        verticalAlignment = Alignment.Bottom,
        horizontalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        for (i in 1..barCount) {
            val active = i <= level
            Box(
                modifier = Modifier
                    .width(4.dp)
                    .height((5 * i + 3).dp)
                    .background(
                        color = if (active) activeColor else inactiveColor,
                        shape = RoundedCornerShape(1.dp),
                    ),
            )
        }
    }
}

/** Texto numérico que sube animado al llegar o cambiar [value] (en vez de aparecer de golpe). */
@Composable
fun AnimatedCountText(
    value: Int,
    modifier: Modifier = Modifier,
    style: TextStyle = MaterialTheme.typography.headlineMedium,
    color: Color = VigiaColors.TextPrimary,
) {
    val animated by animateIntAsState(
        targetValue = value,
        animationSpec = tween(durationMillis = 700),
        label = "vigiaAnimatedCount",
    )
    Text(text = "$animated", modifier = modifier, style = style, color = color)
}

/**
 * Una de las "tiles" del Panel (Equipos / Riesgos o Seguridad / Señal):
 * valor grande arriba, etiqueta chica abajo. [onClick] es opcional (p. ej.
 * con el informe de seguridad disponible, tocar la tile abre esa pantalla —
 * ver `ui/panel/PanelScreen.kt`); sin él, la tile no reacciona a toques,
 * igual que antes.
 */
@Composable
fun StatTile(
    label: String,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null,
    value: (@Composable () -> Unit)? = null,
) {
    // Fusionada (ver AGENTS.md § accesibilidad): sin esto, TalkBack lee el
    // valor y la etiqueta como dos paradas sueltas ("3", luego "Equipos") en
    // vez de una sola ("3, Equipos"), y la tile clickable (Seguridad) no
    // anunciaba que es un botón.
    val merged = Modifier.semantics(mergeDescendants = true) {}
    GlassSurface(modifier = if (onClick != null) modifier.clickable(onClick = onClick).then(merged) else modifier.then(merged)) {
        Column(
            modifier = Modifier.fillMaxWidth().padding(vertical = 14.dp, horizontal = 8.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            value?.invoke()
            Text(
                label.uppercase(),
                style = MaterialTheme.typography.labelSmall,
                color = VigiaColors.TextSecondary,
                textAlign = TextAlign.Center,
            )
        }
    }
}

/** Chip de estado del Panel: "Escaneando… etapa" o "hace 2 min", con un punto de color. */
@Composable
fun StatusChip(text: String, modifier: Modifier = Modifier, dotColor: Color = VigiaColors.RadarCyan) {
    Surface(
        modifier = modifier,
        shape = RoundedCornerShape(50),
        color = VigiaColors.SurfaceElevated,
        border = BorderStroke(1.dp, VigiaColors.Outline),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Box(Modifier.size(6.dp).background(dotColor, CircleShape))
            Text(text, style = MaterialTheme.typography.labelMedium, color = VigiaColors.TextPrimary)
        }
    }
}

/**
 * Anillo de confianza con el porcentaje en el centro: "mini anillo" en listas
 * (tamaño chico) o el anillo grande de la cabecera del Detalle.
 */
@Composable
fun ConfidenceRing(
    confidence: Int,
    modifier: Modifier = Modifier,
    size: Dp = 36.dp,
    strokeWidth: Dp = 3.dp,
    textStyle: TextStyle = VigiaMono.labelSmall,
) {
    val color = when {
        confidence >= 80 -> VigiaColors.RadarCyan
        confidence >= 50 -> VigiaColors.ElectricBlue
        else -> VigiaColors.TextTertiary
    }
    Box(modifier = modifier.size(size), contentAlignment = Alignment.Center) {
        Canvas(modifier = Modifier.size(size)) {
            val stroke = strokeWidth.toPx()
            drawArc(
                color = VigiaColors.Outline,
                startAngle = -90f,
                sweepAngle = 360f,
                useCenter = false,
                style = Stroke(width = stroke),
            )
            drawArc(
                color = color,
                startAngle = -90f,
                sweepAngle = 360f * (confidence.coerceIn(0, 100) / 100f),
                useCenter = false,
                style = Stroke(width = stroke, cap = StrokeCap.Round),
            )
        }
        Text("$confidence%", style = textStyle, color = VigiaColors.TextPrimary)
    }
}

/**
 * "hace un momento" / "hace 50 s" / "hace 2 min" / "hace 3 h": la frase
 * completa, para pegarla después de otro texto ("Visto hace 2 min",
 * "último escaneo hace un momento"). Con "ahora mismo" quedaba "hace ahora mismo".
 */
@Composable
fun agoLabel(relative: RelativeTime): String = when (relative) {
    RelativeTime.JustNow -> stringResource(R.string.time_ago_moment)
    is RelativeTime.Seconds -> stringResource(R.string.panel_status_relative, stringResource(R.string.panel_time_seconds, relative.value.toInt()))
    is RelativeTime.Minutes -> stringResource(R.string.panel_status_relative, stringResource(R.string.panel_time_minutes, relative.value.toInt()))
    is RelativeTime.Hours -> stringResource(R.string.panel_status_relative, stringResource(R.string.panel_time_hours, relative.value.toInt()))
    is RelativeTime.Days -> stringResource(R.string.panel_status_relative, stringResource(R.string.panel_time_days, relative.value.toInt()))
}
