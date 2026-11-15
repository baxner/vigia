package com.baxner.vigia.data.radar

import kotlin.math.abs
import kotlin.math.asin
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.sin

/*
 * Matemáticas puras de la brújula (ver RotationHeadingSource.kt y
 * docs/plan.md, fase 6, paso 2 "Radar con brújula"). Separadas del sensor
 * para poder probarlas con JUnit plano: SensorManager.getOrientation() no
 * sirve para eso (los stubs de Android en pruebas unitarias no calculan
 * nada real, igual que WifiManager.calculateSignalLevel en WifiUtils.kt),
 * así que la extracción de ángulos desde la matriz de rotación se
 * reimplementa a mano aquí con la misma trigonometría que usa la
 * plataforma (verificada contra las fórmulas documentadas de
 * SensorManager.getOrientation: azimuth = atan2(R[1], R[4]),
 * pitch = asin(-R[7])).
 */

/** 0 ≤ grados < 360. */
fun normalizeDegrees(deg: Float): Float {
    val r = deg % 360f
    return if (r < 0f) r + 360f else r
}

/**
 * Rumbo (grados desde el norte) a partir de las componentes Este/Norte de
 * un eje del teléfono, ya ubicadas en la matriz de rotación `R` de 9
 * valores (`[Este, Norte, Arriba] = R · [x, y, z]_teléfono`, el mismo
 * convenio que `SensorManager.getRotationMatrix`). [negate] voltea el eje
 * (para leer la dirección opuesta a la que da la matriz).
 */
private fun headingFromAxis(r: FloatArray, eastIndex: Int, northIndex: Int, negate: Boolean): Float {
    val sign = if (negate) -1.0 else 1.0
    val east = sign * r[eastIndex]
    val north = sign * r[northIndex]
    return normalizeDegrees(Math.toDegrees(atan2(east, north)).toFloat())
}

/**
 * Rumbo del borde superior del teléfono (columna Y de `R`: índices 1 y 4).
 * Es lo que da `SensorManager.getOrientation` con el teléfono plano (boca
 * arriba); vertical, el borde superior apunta casi al cielo y este rumbo
 * se vuelve ruido (ver [verticalAzimuthDeg]).
 */
fun flatAzimuthDeg(r: FloatArray): Float = headingFromAxis(r, eastIndex = 1, northIndex = 4, negate = false)

/**
 * Rumbo hacia donde apunta la cámara trasera: el eje Z de la matriz
 * (índices 2 y 5) sale de la *pantalla* hacia quien la mira, así que la
 * parte de atrás —la cámara— apunta al revés (de ahí el signo invertido).
 * Sirve con el teléfono casi vertical, mirándolo de frente (ver
 * [postureAwareAzimuthDeg]).
 */
fun verticalAzimuthDeg(r: FloatArray): Float = headingFromAxis(r, eastIndex = 2, northIndex = 5, negate = true)

/**
 * Cabeceo en grados (misma fórmula que `SensorManager.getOrientation`:
 * `pitch = asin(-R[7])`). Solo se usa para decidir la postura
 * ([postureAwareAzimuthDeg]), no como ángulo absoluto.
 */
fun pitchDeg(r: FloatArray): Float = Math.toDegrees(asin((-r[7]).toDouble().coerceIn(-1.0, 1.0))).toFloat()

/**
 * Rumbo según la postura del teléfono (docs/plan.md, fase 6): con el
 * teléfono casi vertical (|cabeceo| > [verticalThresholdDeg], 45° por
 * encargo) se usa hacia dónde apunta la cámara trasera; si no, el borde
 * superior de la pantalla (lo normal con el teléfono plano).
 */
fun postureAwareAzimuthDeg(r: FloatArray, verticalThresholdDeg: Float = 45f): Float =
    if (abs(pitchDeg(r)) > verticalThresholdDeg) verticalAzimuthDeg(r) else flatAzimuthDeg(r)

/**
 * Suavizado circular (exponencial, sin saltos en 359°→0°): media pesada de
 * los vectores unitarios (cos, sin) de cada ángulo, no de los grados
 * crudos — promediar 359 y 1 a lo crudo da 180 (el lado opuesto);
 * promediar los vectores da ~0, que es lo correcto. [alpha] en (0, 1]: 1
 * ignora el historial (solo la lectura nueva); valores chicos suavizan más
 * pero siguen más lento un giro real. `null` en [previousDeg] (primera
 * lectura) devuelve [nextDeg] tal cual.
 */
fun smoothAzimuthDeg(previousDeg: Float?, nextDeg: Float, alpha: Float): Float {
    if (previousDeg == null) return normalizeDegrees(nextDeg)
    val prevRad = Math.toRadians(previousDeg.toDouble())
    val nextRad = Math.toRadians(nextDeg.toDouble())
    val x = (1 - alpha) * cos(prevRad) + alpha * cos(nextRad)
    val y = (1 - alpha) * sin(prevRad) + alpha * sin(nextRad)
    return normalizeDegrees(Math.toDegrees(atan2(y, x)).toFloat())
}

/**
 * Par de ejes para `SensorManager.remapCoordinateSystem` que compensa la
 * rotación de pantalla (tabla estándar de Android: el remapeo solo
 * intercambia/voltea X e Y entre sí; el eje Z —hacia/desde la pantalla,
 * que usa [verticalAzimuthDeg]— no cambia con la rotación de pantalla).
 * [surfaceRotation] es `Surface.ROTATION_0/90/180/270`; cualquier otro
 * valor se trata como 0° (sin compensar) en vez de fallar.
 */
fun screenRemapAxes(surfaceRotation: Int): Pair<Int, Int> = when (surfaceRotation) {
    1 -> AXIS_Y to AXIS_MINUS_X // ROTATION_90
    2 -> AXIS_MINUS_X to AXIS_MINUS_Y // ROTATION_180
    3 -> AXIS_MINUS_Y to AXIS_X // ROTATION_270
    else -> AXIS_X to AXIS_Y // ROTATION_0 (o desconocida)
}

// Mismos valores que android.hardware.SensorManager.AXIS_*; copiados como
// constantes para que esta función siga siendo Kotlin puro (sin import de
// Android) y se pueda probar en la JVM sin stubs.
private const val AXIS_X = 1
private const val AXIS_Y = 2
private const val AXIS_MINUS_X = 129
private const val AXIS_MINUS_Y = 130
