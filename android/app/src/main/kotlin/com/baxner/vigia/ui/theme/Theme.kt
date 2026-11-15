package com.baxner.vigia.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

/**
 * El [androidx.compose.material3.ColorScheme] de "Radar Nocturno", construido
 * sobre [VigiaColors]. Siempre oscuro a propósito: esta identidad no tiene
 * versión clara ni sigue el color dinámico de Material You (ver [VigiaTheme]).
 */
private val VigiaDarkColorScheme = darkColorScheme(
    primary = VigiaColors.RadarCyan,
    onPrimary = VigiaColors.Background,
    primaryContainer = VigiaColors.SurfaceElevated,
    onPrimaryContainer = VigiaColors.RadarCyan,
    inversePrimary = VigiaColors.Background,

    secondary = VigiaColors.ElectricBlue,
    onSecondary = VigiaColors.Background,
    secondaryContainer = VigiaColors.SurfaceElevated,
    onSecondaryContainer = VigiaColors.ElectricBlue,

    tertiary = VigiaColors.Amber,
    onTertiary = VigiaColors.Background,
    tertiaryContainer = VigiaColors.SurfaceElevated,
    onTertiaryContainer = VigiaColors.Amber,

    error = VigiaColors.Risk,
    onError = VigiaColors.Background,
    errorContainer = VigiaColors.SurfaceElevated,
    onErrorContainer = VigiaColors.Risk,

    background = VigiaColors.Background,
    onBackground = VigiaColors.TextPrimary,

    surface = VigiaColors.Surface,
    onSurface = VigiaColors.TextPrimary,
    surfaceVariant = VigiaColors.SurfaceElevated,
    onSurfaceVariant = VigiaColors.TextSecondary,

    surfaceContainerLowest = VigiaColors.Background,
    surfaceContainerLow = VigiaColors.Surface,
    surfaceContainer = VigiaColors.Surface,
    surfaceContainerHigh = VigiaColors.SurfaceElevated,
    surfaceContainerHighest = VigiaColors.SurfaceElevated,
    surfaceDim = VigiaColors.Background,
    surfaceBright = VigiaColors.SurfaceElevated,

    outline = VigiaColors.Outline,
    outlineVariant = VigiaColors.Outline,

    inverseSurface = VigiaColors.TextPrimary,
    inverseOnSurface = VigiaColors.Background,
    scrim = Color.Black,
)

/** Esquinas de 20 dp en las tarjetas "vidrio" y progresivamente más chicas hacia abajo. */
private val VigiaShapes = Shapes(
    extraSmall = RoundedCornerShape(6.dp),
    small = RoundedCornerShape(10.dp),
    medium = RoundedCornerShape(14.dp),
    large = RoundedCornerShape(20.dp),
    extraLarge = RoundedCornerShape(28.dp),
)

/**
 * Tema de Vigía: "Radar Nocturno", siempre oscuro. A propósito ignora
 * [androidx.compose.foundation.isSystemInDarkTheme] y el color dinámico de
 * Material You (Android 12+): es una identidad propia, no la paleta del
 * fondo de pantalla del usuario.
 */
@Composable
fun VigiaTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = VigiaDarkColorScheme,
        typography = VigiaTypography,
        shapes = VigiaShapes,
        content = content,
    )
}
