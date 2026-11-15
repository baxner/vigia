package com.baxner.vigia.data.radar

import kotlin.math.abs
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Matemáticas puras de la brújula con datos sintéticos (ver HeadingMath.kt). */
class HeadingMathTest {

    private fun angleError(a: Float, b: Float): Float {
        val d = abs(a - b) % 360f
        return if (d > 180f) 360f - d else d
    }

    /** Matriz mínima: solo rellena los índices que leen las funciones (1, 4 para Y; 2, 5 para Z; 7 para cabeceo). */
    private fun matrix(y1: Float = 0f, y4: Float = 1f, z2: Float = 0f, z5: Float = -1f, p7: Float = 0f) =
        FloatArray(9).also { it[1] = y1; it[4] = y4; it[2] = z2; it[5] = z5; it[7] = p7 }

    // ---- normalizeDegrees ----

    @Test
    fun `normaliza a 0-360`() {
        assertEquals(0f, normalizeDegrees(360f), 0.001f)
        assertEquals(350f, normalizeDegrees(-10f), 0.001f)
        assertEquals(10f, normalizeDegrees(370f), 0.001f)
        assertEquals(0f, normalizeDegrees(0f), 0.001f)
    }

    // ---- flatAzimuthDeg / verticalAzimuthDeg / pitchDeg ----

    @Test
    fun `rumbo plano sigue el eje Y del telefono (borde superior)`() {
        assertEquals(0f, flatAzimuthDeg(matrix(y1 = 0f, y4 = 1f)), 0.01f) // Y apunta al norte
        assertEquals(90f, flatAzimuthDeg(matrix(y1 = 1f, y4 = 0f)), 0.01f) // Y apunta al este
        assertEquals(180f, flatAzimuthDeg(matrix(y1 = 0f, y4 = -1f)), 0.01f) // Y apunta al sur
        assertEquals(270f, flatAzimuthDeg(matrix(y1 = -1f, y4 = 0f)), 0.01f) // Y apunta al oeste
    }

    @Test
    fun `rumbo vertical sigue la camara trasera, no el eje Y`() {
        // Eje Z (de la pantalla hacia el usuario) apuntando al sur -> la cámara trasera apunta al norte.
        assertEquals(0f, verticalAzimuthDeg(matrix(z2 = 0f, z5 = -1f)), 0.01f)
        // Eje Z apuntando al oeste -> la cámara trasera apunta al este.
        assertEquals(90f, verticalAzimuthDeg(matrix(z2 = -1f, z5 = 0f)), 0.01f)
    }

    @Test
    fun `el cabeceo usa la formula de SensorManager (asin de -R7)`() {
        assertEquals(0f, pitchDeg(matrix(p7 = 0f)), 0.01f)
        assertEquals(90f, pitchDeg(matrix(p7 = -1f)), 0.01f)
        assertEquals(-90f, pitchDeg(matrix(p7 = 1f)), 0.01f)
    }

    // ---- postureAwareAzimuthDeg ----

    @Test
    fun `con el telefono plano usa el rumbo del borde superior`() {
        // Y apunta al este (90°), Z apunta al sur (vertical daría 0°); cabeceo 0° = plano.
        val r = matrix(y1 = 1f, y4 = 0f, z2 = 0f, z5 = -1f, p7 = 0f)
        assertEquals(90f, postureAwareAzimuthDeg(r), 0.01f)
    }

    @Test
    fun `con el telefono casi vertical usa hacia donde apunta la camara`() {
        // Mismos ejes que la prueba anterior, pero con cabeceo de 90° (vertical).
        val r = matrix(y1 = 1f, y4 = 0f, z2 = 0f, z5 = -1f, p7 = -1f)
        assertEquals(0f, postureAwareAzimuthDeg(r), 0.01f)
    }

    @Test
    fun `el cambio de postura ocurre justo en el umbral de 45 grados`() {
        val r = matrix(y1 = 1f, y4 = 0f, z2 = 0f, z5 = -1f, p7 = -1f) // cabeceo 90°
        assertEquals(0f, postureAwareAzimuthDeg(r, verticalThresholdDeg = 45f), 0.01f)
        assertEquals(90f, postureAwareAzimuthDeg(r, verticalThresholdDeg = 95f), 0.01f) // 90° ya no pasa un umbral de 95°
    }

    // ---- smoothAzimuthDeg ----

    @Test
    fun `sin lectura previa devuelve la nueva tal cual`() {
        assertEquals(42f, smoothAzimuthDeg(null, 42f, alpha = 0.3f), 0.01f)
    }

    @Test
    fun `cruzar 359 a 0 no da un salto al lado opuesto`() {
        val result = smoothAzimuthDeg(previousDeg = 359f, nextDeg = 1f, alpha = 0.5f)
        assertTrue("debía quedar cerca de 0, dio $result", angleError(result, 0f) < 1f)
    }

    @Test
    fun `alpha 1 ignora el historial`() {
        assertEquals(200f, smoothAzimuthDeg(previousDeg = 50f, nextDeg = 200f, alpha = 1f), 0.01f)
    }

    @Test
    fun `alpha chico se mueve poco hacia la lectura nueva`() {
        val result = smoothAzimuthDeg(previousDeg = 0f, nextDeg = 90f, alpha = 0.1f)
        assertTrue("debía quedarse cerca de 0, dio $result", angleError(result, 0f) < angleError(result, 90f))
    }

    // ---- screenRemapAxes ----

    @Test
    fun `remapeo de ejes por rotacion de pantalla`() {
        assertEquals(1 to 2, screenRemapAxes(0)) // ROTATION_0: X, Y tal cual
        assertEquals(2 to 129, screenRemapAxes(1)) // ROTATION_90
        assertEquals(129 to 130, screenRemapAxes(2)) // ROTATION_180
        assertEquals(130 to 1, screenRemapAxes(3)) // ROTATION_270
        assertEquals(1 to 2, screenRemapAxes(99)) // valor desconocido: sin compensar
    }
}
