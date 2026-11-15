package com.baxner.vigia.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import com.baxner.vigia.R

/**
 * Las tres familias de "Radar Nocturno", todas OFL (licencias en
 * `assets/licenses/`, fuentes en `res/font/`, instanciadas como estáticas
 * desde los variable fonts de github.com/google/fonts):
 * - Space Grotesk: títulos y números grandes.
 * - Inter: texto.
 * - JetBrains Mono: IPs, MAC, puertos, dBm.
 */
val SpaceGrotesk = FontFamily(
    Font(R.font.space_grotesk_regular, FontWeight.Normal),
    Font(R.font.space_grotesk_medium, FontWeight.Medium),
    Font(R.font.space_grotesk_bold, FontWeight.Bold),
)

val Inter = FontFamily(
    Font(R.font.inter_regular, FontWeight.Normal),
    Font(R.font.inter_medium, FontWeight.Medium),
    Font(R.font.inter_semibold, FontWeight.SemiBold),
    Font(R.font.inter_bold, FontWeight.Bold),
)

val JetBrainsMono = FontFamily(
    Font(R.font.jetbrains_mono_regular, FontWeight.Normal),
    Font(R.font.jetbrains_mono_medium, FontWeight.Medium),
    Font(R.font.jetbrains_mono_bold, FontWeight.Bold),
)

/**
 * Tipografía M3: Space Grotesk en display/headline/titleLarge (títulos y
 * números grandes), Inter en el resto (cuerpo, etiquetas).
 */
val VigiaTypography = Typography().let { base ->
    base.copy(
        displayLarge = base.displayLarge.copy(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Bold),
        displayMedium = base.displayMedium.copy(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Bold),
        displaySmall = base.displaySmall.copy(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Medium),
        headlineLarge = base.headlineLarge.copy(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Bold),
        headlineMedium = base.headlineMedium.copy(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Medium),
        headlineSmall = base.headlineSmall.copy(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Medium),
        titleLarge = base.titleLarge.copy(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Medium),
        titleMedium = base.titleMedium.copy(fontFamily = Inter, fontWeight = FontWeight.SemiBold),
        titleSmall = base.titleSmall.copy(fontFamily = Inter, fontWeight = FontWeight.SemiBold),
        bodyLarge = base.bodyLarge.copy(fontFamily = Inter),
        bodyMedium = base.bodyMedium.copy(fontFamily = Inter),
        bodySmall = base.bodySmall.copy(fontFamily = Inter),
        labelLarge = base.labelLarge.copy(fontFamily = Inter, fontWeight = FontWeight.Medium),
        labelMedium = base.labelMedium.copy(fontFamily = Inter, fontWeight = FontWeight.Medium),
        labelSmall = base.labelSmall.copy(fontFamily = Inter, fontWeight = FontWeight.Medium),
    )
}

/**
 * Estilos en JetBrains Mono para datos técnicos (IP, MAC, puertos, dBm):
 * fuera de la escala de [Typography] porque M3 no tiene un slot "mono".
 */
object VigiaMono {
    val large: TextStyle = TextStyle(fontFamily = JetBrainsMono, fontSize = 22.sp, fontWeight = FontWeight.Medium)
    val body: TextStyle = TextStyle(fontFamily = JetBrainsMono, fontSize = 14.sp, fontWeight = FontWeight.Normal)
    val bodyMedium: TextStyle = TextStyle(fontFamily = JetBrainsMono, fontSize = 14.sp, fontWeight = FontWeight.Medium)
    val label: TextStyle = TextStyle(fontFamily = JetBrainsMono, fontSize = 12.sp, fontWeight = FontWeight.Normal)
    val labelSmall: TextStyle = TextStyle(fontFamily = JetBrainsMono, fontSize = 11.sp, fontWeight = FontWeight.Medium)
}
