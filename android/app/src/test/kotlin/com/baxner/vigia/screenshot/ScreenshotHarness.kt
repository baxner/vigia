package com.baxner.vigia.screenshot

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Surface
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.baxner.vigia.ui.theme.VigiaTheme

/**
 * Tamaño de teléfono para las capturas de revisión visual (ver el encargo
 * "Radar Nocturno"). Se fija con un Box de este tamaño exacto, no solo con
 * los qualifiers de Robolectric: sin él, onRoot() mide el contenido real de
 * la pantalla (p. ej. todo el LazyColumn sin recortar), no un viewport fijo.
 */
private val PhoneWidth = 412.dp
private val PhoneHeight = 915.dp

/**
 * Envuelve [content] con el tema y el tamaño de pantalla fijo de las
 * capturas. [height] por defecto es el alto real de teléfono; algunas
 * pantallas con una sección larga (p. ej. Detalle con cronología completa)
 * piden un lienzo más alto para que no se corte fuera del viewport fijo.
 *
 * [fontScale] simula el ajuste de letra grande de Android (Ajustes →
 * Pantalla → Tamaño de letra): 1.5f equivale a "grande". Un intento anterior
 * probó subiendo [androidx.compose.material3.LocalTextStyle], pero eso no
 * escala nada — casi todo el texto de la app usa estilos de
 * `MaterialTheme.typography` con su propio `fontSize` explícito, que pisa el
 * `LocalTextStyle` heredado en vez de combinarse con él. La manera real de
 * emular el ajuste del sistema es cambiar la densidad de fuente
 * ([Density.fontScale]), que es justo lo que lee cada `sp` al medir texto,
 * sin tocar la densidad de los `dp` (el ancho de tarjetas, iconos, etc. no
 * debe moverse solo porque la letra creció).
 */
@Composable
fun VigiaScreenshotFrame(height: Dp = PhoneHeight, fontScale: Float = 1f, content: @Composable () -> Unit) {
    VigiaTheme {
        Box(modifier = Modifier.size(PhoneWidth, height)) {
            Surface(modifier = Modifier.fillMaxSize()) {
                if (fontScale == 1f) {
                    content()
                } else {
                    val density = LocalDensity.current
                    CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                        content()
                    }
                }
            }
        }
    }
}
