package com.baxner.vigia.ui.theme

import androidx.compose.ui.graphics.Color

/**
 * Paleta "Radar Nocturno": la identidad visual de Vigía. Siempre oscura (no
 * hay versión clara ni color dinámico de Material You): un monitor de red
 * elegante y tecnológico, no un andamio genérico de Material.
 *
 * Estos son los tokens "crudos", usados tanto para construir el
 * [androidx.compose.material3.ColorScheme] en [VigiaTheme] como directo en
 * dibujo custom (el radar en Canvas, las curvas del analizador Wi-Fi) donde
 * pedir el color vía `MaterialTheme.colorScheme` sería indirecto.
 */
object VigiaColors {
    // Fondo y superficies.
    val Background = Color(0xFF070B12)
    val Surface = Color(0xFF0D1420)
    val SurfaceElevated = Color(0xFF121B2B)
    val Outline = Color(0xFF1E2A3D)

    // Acentos.
    /** Primario: cian de radar. */
    val RadarCyan = Color(0xFF34E3C2)
    /** Secundario: azul eléctrico. */
    val ElectricBlue = Color(0xFF4C8DFF)
    /** Atención / sin identificar. */
    val Amber = Color(0xFFFFB547)
    /** Riesgo. */
    val Risk = Color(0xFFFF5D6C)
    /** Ok / seguro. */
    val Ok = Color(0xFF3DDC84)

    // Texto.
    val TextPrimary = Color(0xFFE6EEF8)
    val TextSecondary = Color(0xFF8A9BB4)
    val TextTertiary = Color(0xFF5B6B82)
}
