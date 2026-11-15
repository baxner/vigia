package com.baxner.vigia.ui.panel

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.State
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.baxner.vigia.R
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.ui.devices.effectiveCoarseType
import com.baxner.vigia.ui.devices.radarAngles
import com.baxner.vigia.ui.devices.radarRing
import com.baxner.vigia.ui.devices.statusColorFor
import com.baxner.vigia.ui.theme.VigiaColors
import kotlin.math.cos
import kotlin.math.min
import kotlin.math.sin

/** Fracción del radio máximo para cada anillo (0 = centro, el router). */
private val RingRadiusFraction = listOf(0f, 0.40f, 0.67f, 0.94f)

/**
 * El héroe del Panel: un radar en Canvas. Círculos concéntricos y retícula
 * siempre visibles (atenuados en reposo); un barrido giratorio con estela
 * mientras escanea (quieto si [scanning] es falso o si [reduceMotion] lo
 * pide); los equipos como puntos que aparecen con un fundido al llegar el
 * resultado. Tocarlo navega a Dispositivos.
 */
@Composable
fun RadarHero(
    devices: List<Device>,
    scanning: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    reduceMotion: Boolean = false,
    // Solo los equipos con algo personalizado (ver PanelScreen): el tipo
    // elegido a mano manda sobre el del motor, también aquí.
    customByIp: Map<String, DeviceCustomization> = emptyMap(),
) {
    // Fundido de entrada: una nueva lista de equipos (llegó un resultado nuevo)
    // reinicia el Animatable de 0 a 1.
    val appear = remember(devices) { Animatable(if (devices.isEmpty()) 1f else 0f) }
    LaunchedEffect(devices) { appear.animateTo(1f, tween(durationMillis = 650)) }
    val appearAlpha = appear.value

    val sweepAngleState: State<Float> = if (scanning && !reduceMotion) {
        val transition = rememberInfiniteTransition(label = "radarSweep")
        transition.animateFloat(
            initialValue = 0f,
            targetValue = 360f,
            animationSpec = infiniteRepeatable(
                animation = tween(durationMillis = 2600, easing = LinearEasing),
                repeatMode = RepeatMode.Restart,
            ),
            label = "radarSweepAngle",
        )
    } else {
        remember { mutableFloatStateOf(40f) }
    }
    val sweepAngle = sweepAngleState.value

    val positioned = remember(devices, customByIp) {
        val ringOf = { d: Device -> d.effectiveCoarseType(customByIp).radarRing() }
        val angles = radarAngles(devices, ringOf)
        devices.map { device -> Triple(device, angles[device.ip] ?: 0f, ringOf(device)) }
    }

    // Descripción resumida para TalkBack (ver AGENTS.md § accesibilidad): no
    // lee equipo por equipo, solo cuántos hay (con el plural correcto) o que
    // está escaneando.
    val radarDescription = if (scanning) {
        stringResource(R.string.radar_description_scanning)
    } else {
        pluralStringResource(R.plurals.radar_description_devices, devices.size, devices.size)
    }
    Canvas(
        modifier = modifier
            .fillMaxWidth()
            .aspectRatio(1f)
            .clip(RoundedCornerShape(20.dp))
            .clickable(
                interactionSource = remember { MutableInteractionSource() },
                indication = null,
                role = Role.Button,
                onClick = onClick,
            )
            .semantics { contentDescription = radarDescription },
    ) {
        val maxRadius = min(size.width, size.height) / 2f * 0.9f
        val center = Offset(size.width / 2f, size.height / 2f)
        val restAlpha = if (scanning) 1f else 0.55f

        // Resplandor de fondo, muy tenue.
        drawCircle(
            brush = Brush.radialGradient(
                colors = listOf(VigiaColors.SurfaceElevated.copy(alpha = 0.7f), Color.Transparent),
                center = center,
                radius = maxRadius * 1.15f,
            ),
            radius = maxRadius * 1.15f,
            center = center,
        )

        // Anillos concéntricos.
        RingRadiusFraction.drop(1).forEach { fraction ->
            drawCircle(
                color = VigiaColors.RadarCyan.copy(alpha = 0.16f * restAlpha),
                radius = maxRadius * fraction,
                center = center,
                style = Stroke(width = 1.dp.toPx()),
            )
        }
        // Retícula (cruz).
        drawLine(
            color = VigiaColors.RadarCyan.copy(alpha = 0.10f * restAlpha),
            start = Offset(center.x - maxRadius, center.y),
            end = Offset(center.x + maxRadius, center.y),
            strokeWidth = 1.dp.toPx(),
        )
        drawLine(
            color = VigiaColors.RadarCyan.copy(alpha = 0.10f * restAlpha),
            start = Offset(center.x, center.y - maxRadius),
            end = Offset(center.x, center.y + maxRadius),
            strokeWidth = 1.dp.toPx(),
        )

        // Barrido giratorio con estela en degradado, solo mientras escanea.
        if (scanning) {
            rotate(degrees = sweepAngle, pivot = center) {
                drawCircle(
                    brush = Brush.sweepGradient(
                        colorStops = arrayOf(
                            0f to Color.Transparent,
                            0.78f to Color.Transparent,
                            0.92f to VigiaColors.RadarCyan.copy(alpha = 0.10f),
                            0.985f to VigiaColors.RadarCyan.copy(alpha = 0.42f),
                            1f to Color.Transparent,
                        ),
                        center = center,
                    ),
                    radius = maxRadius,
                    center = center,
                )
            }
        }

        // Equipos: ángulo por último octeto de IP, anillo por tipo.
        positioned.forEach { (device, angleDeg, ring) ->
            val radiusFraction = RingRadiusFraction.getOrElse(ring) { RingRadiusFraction.last() }
            val pos = if (radiusFraction == 0f) {
                center
            } else {
                val rad = Math.toRadians(angleDeg.toDouble())
                Offset(
                    x = center.x + maxRadius * radiusFraction * cos(rad).toFloat(),
                    y = center.y + maxRadius * radiusFraction * sin(rad).toFloat(),
                )
            }
            val dotColor = statusColorFor(device, device.effectiveCoarseType(customByIp))
            val dotRadius = if (ring == 0) 7.dp.toPx() else 5.dp.toPx()

            drawCircle(
                brush = Brush.radialGradient(
                    colors = listOf(dotColor.copy(alpha = 0.38f * appearAlpha), Color.Transparent),
                    center = pos,
                    radius = dotRadius * 3.2f,
                ),
                radius = dotRadius * 3.2f,
                center = pos,
            )
            drawCircle(color = dotColor.copy(alpha = appearAlpha), radius = dotRadius, center = pos)
            drawCircle(
                color = VigiaColors.Background.copy(alpha = appearAlpha),
                radius = dotRadius * 0.4f,
                center = pos,
            )
        }
    }
}
