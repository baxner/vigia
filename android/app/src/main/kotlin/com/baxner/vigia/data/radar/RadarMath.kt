package com.baxner.vigia.data.radar

import kotlin.math.abs
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.hypot
import kotlin.math.pow
import kotlin.math.sin
import kotlin.math.sqrt

/*
 * Matemáticas puras de "Buscar señal" (fase 6, ver docs/plan.md): sin
 * Android ni radios, para probarlas con datos sintéticos.
 */

/**
 * Filtro de Kalman de una dimensión para la señal (RSSI): suaviza el ruido
 * (±3–6 dB entre lecturas) sin el retraso de un promedio largo.
 * [processNoise] alto sigue más rápido los cambios reales (caminar);
 * [measurementNoise] alto desconfía más de cada lectura.
 */
class RssiFilter(
    private val processNoise: Double = 0.1,
    private val measurementNoise: Double = 9.0,
) {
    private var estimate: Double? = null
    private var errorCovariance = 0.0

    /** El valor suavizado, o null antes de la primera lectura. */
    val value: Double? get() = estimate

    fun update(rssi: Double): Double {
        val current = estimate
        if (current == null) {
            estimate = rssi
            errorCovariance = measurementNoise
            return rssi
        }
        val predictedCovariance = errorCovariance + processNoise
        val gain = predictedCovariance / (predictedCovariance + measurementNoise)
        val next = current + gain * (rssi - current)
        errorCovariance = (1 - gain) * predictedCovariance
        estimate = next
        return next
    }

    fun reset() {
        estimate = null
        errorCovariance = 0.0
    }

    companion object {
        /** Bluetooth: muchas lecturas por segundo y ruidosas; ~1 s de retraso a 10 lecturas/s. */
        fun forBle(): RssiFilter = RssiFilter(processNoise = 0.1, measurementNoise = 9.0)

        /** Wi-Fi: una lectura cada 1–3 s; sigue más rápido para no ir varios pasos atrás. */
        fun forWifi(): RssiFilter = RssiFilter(processNoise = 1.5, measurementNoise = 9.0)
    }
}

/** Distancia aproximada, como rango: por señal no se puede dar un número exacto. */
data class DistanceRange(val minMeters: Double, val typicalMeters: Double, val maxMeters: Double)

/**
 * Modelo de pérdida log-distancia: rssi = ref1m − 10·n·log10(d). Dentro de una
 * casa n va de ~2 (vista directa) a ~3.5 (paredes): de ahí el rango.
 */
object DistanceModel {
    private const val PathLossMin = 2.0
    private const val PathLossTypical = 2.7
    private const val PathLossMax = 3.5
    private const val MinMeters = 0.3
    private const val MaxMeters = 60.0

    /** Señal esperada a 1 m del objetivo, en dBm. */
    fun referenceAt1m(target: SignalTarget): Double = when (target) {
        // Un router de casa a 1 m ronda los −38 dBm en 2.4 GHz; 5 y 6 GHz pierden más.
        is SignalTarget.WifiAp -> if (target.frequencyMhz >= 4900) -43.0 else -38.0
        // Tx Power Level es la potencia a la salida de la antena; a 1 m se pierden ~41 dB en 2.4 GHz.
        is SignalTarget.Ble -> target.txPowerDbm?.let { it - 41.0 } ?: -62.0
    }

    fun estimate(rssi: Double, referenceAt1m: Double): DistanceRange {
        fun distance(n: Double) = 10.0.pow((referenceAt1m - rssi) / (10 * n)).coerceIn(MinMeters, MaxMeters)
        // Más allá de 1 m, un exponente alto da la distancia más corta; a menos de 1 m (señal
        // más fuerte que la referencia) es al revés. Se ordena para que min ≤ max siempre.
        val a = distance(PathLossMax)
        val b = distance(PathLossMin)
        return DistanceRange(minMeters = minOf(a, b), typicalMeters = distance(PathLossTypical), maxMeters = maxOf(a, b))
    }
}

enum class Trend { CLOSER, FARTHER, STEADY }

/**
 * Tendencia de la señal suavizada: compara contra la lectura de hace
 * [windowMillis] y exige ±[thresholdDb] para no saltar con el ruido. Para
 * Wi-Fi, con lecturas lentas, conviene una ventana más larga (~8 s).
 */
class TrendDetector(private val windowMillis: Long = 3_000, private val thresholdDb: Double = 2.0) {
    private val history = ArrayDeque<Pair<Long, Double>>()

    fun update(atMillis: Long, smoothedRssi: Double): Trend {
        history.addLast(atMillis to smoothedRssi)
        // Se conserva una lectura de hace al menos windowMillis como referencia.
        while (history.size > 2 && atMillis - history[1].first >= windowMillis) history.removeFirst()
        val reference = history.first()
        if (atMillis - reference.first < windowMillis) return Trend.STEADY
        val delta = smoothedRssi - reference.second
        return when {
            delta >= thresholdDb -> Trend.CLOSER
            delta <= -thresholdDb -> Trend.FARTHER
            else -> Trend.STEADY
        }
    }

    fun reset() = history.clear()
}

/** Una lectura de la vuelta guiada: rumbo del teléfono y señal en ese momento. */
data class AzimuthRssi(val azimuthDeg: Double, val rssi: Double)

enum class BearingConfidence { HIGH, MEDIUM, INCONCLUSIVE }

data class BearingEstimate(
    /** Hacia dónde está el objetivo (grados desde el norte): el rumbo de la señal más fuerte. */
    val bearingDeg: Double,
    /** Diferencia entre la señal de frente y la de espalda, en dB. */
    val swingDb: Double,
    val confidence: BearingConfidence,
)

/**
 * "Vuelta guiada": con el teléfono frente al pecho, el cuerpo tapa la señal de
 * lo que queda a la espalda (5–10 dB). Se ajusta rssi = a + b·cos θ + c·sin θ
 * por mínimos cuadrados y el máximo del coseno, atan2(c, b), apunta al
 * objetivo. Cada sector de 45° pesa lo mismo aunque el usuario se quede más
 * tiempo mirando hacia un lado.
 */
object BearingEstimator {
    const val Sectors = 8
    private const val SectorWidth = 360.0 / Sectors

    fun sectorOf(azimuthDeg: Double): Int = (normalize(azimuthDeg) / SectorWidth).toInt().coerceIn(0, Sectors - 1)

    /** Lecturas por sector, para el anillo que se va llenando. */
    fun sectorCounts(samples: List<AzimuthRssi>): IntArray =
        IntArray(Sectors).also { counts -> samples.forEach { counts[sectorOf(it.azimuthDeg)]++ } }

    /** Null hasta que cada sector tenga al menos [minPerSector] lecturas (vuelta completa). */
    fun estimate(samples: List<AzimuthRssi>, minPerSector: Int = 2): BearingEstimate? {
        val counts = sectorCounts(samples)
        if (counts.any { it < minPerSector }) return null

        var sw = 0.0
        var sc = 0.0
        var ss = 0.0
        var scc = 0.0
        var sss = 0.0
        var scs = 0.0
        var sr = 0.0
        var src = 0.0
        var srs = 0.0
        for (s in samples) {
            val w = 1.0 / counts[sectorOf(s.azimuthDeg)]
            val th = Math.toRadians(s.azimuthDeg)
            val c = cos(th)
            val sn = sin(th)
            sw += w; sc += w * c; ss += w * sn
            scc += w * c * c; sss += w * sn * sn; scs += w * c * sn
            sr += w * s.rssi; src += w * s.rssi * c; srs += w * s.rssi * sn
        }
        val solution = solve3(
            arrayOf(doubleArrayOf(sw, sc, ss), doubleArrayOf(sc, scc, scs), doubleArrayOf(ss, scs, sss)),
            doubleArrayOf(sr, src, srs),
        ) ?: return null
        val a = solution[0]
        val b = solution[1]
        val c = solution[2]
        val amplitude = hypot(b, c)

        // Ruido: cuánto se apartan las lecturas del coseno ajustado.
        var squaredError = 0.0
        for (s in samples) {
            val w = 1.0 / counts[sectorOf(s.azimuthDeg)]
            val th = Math.toRadians(s.azimuthDeg)
            squaredError += w * (s.rssi - (a + b * cos(th) + c * sin(th))).pow(2)
        }
        val noise = sqrt(squaredError / sw)
        // Error típico de la amplitud: con más lecturas, el mismo ruido pesa menos.
        val amplitudeError = noise * sqrt(2.0 / samples.size)
        val swing = 2 * amplitude
        val confidence = when {
            swing >= 6.0 && amplitude >= 4 * amplitudeError -> BearingConfidence.HIGH
            swing >= 3.0 && amplitude >= 2.5 * amplitudeError -> BearingConfidence.MEDIUM
            else -> BearingConfidence.INCONCLUSIVE
        }
        return BearingEstimate(normalize(Math.toDegrees(atan2(c, b))), swing, confidence)
    }

    private fun normalize(deg: Double): Double = ((deg % 360) + 360) % 360

    /** Gauss con pivoteo parcial; null si el sistema es singular. */
    private fun solve3(m: Array<DoubleArray>, v: DoubleArray): DoubleArray? {
        val n = 3
        val a = Array(n) { i -> m[i].copyOf() + v[i] }
        for (col in 0 until n) {
            val pivot = (col until n).maxBy { abs(a[it][col]) }
            if (abs(a[pivot][col]) < 1e-12) return null
            val tmp = a[col]; a[col] = a[pivot]; a[pivot] = tmp
            for (row in col + 1 until n) {
                val f = a[row][col] / a[col][col]
                for (k in col..n) a[row][k] -= f * a[col][k]
            }
        }
        val x = DoubleArray(n)
        for (i in n - 1 downTo 0) {
            var sum = a[i][n]
            for (k in i + 1 until n) sum -= a[i][k] * x[k]
            x[i] = sum / a[i][i]
        }
        return x
    }
}

/** Ritmo del "Geiger": cuántos ms entre clics según la señal. */
object Geiger {
    const val SlowestMillis = 1_200L
    const val FastestMillis = 80L

    /** Lento con señal débil (≤ −95 dBm), rápido con señal fuerte (≥ −35 dBm), en curva exponencial: cada dB se nota igual. */
    fun intervalMillis(smoothedRssi: Double): Long {
        val t = ((smoothedRssi + 95.0) / 60.0).coerceIn(0.0, 1.0)
        return (SlowestMillis * (FastestMillis.toDouble() / SlowestMillis).pow(t)).toLong()
    }
}

/**
 * Parentesco de MAC: muchos equipos IoT (ESP32 y similares) derivan la
 * dirección Bluetooth de la de Wi-Fi (mismos 5 bytes, el último +1 o +2).
 */
object MacKinship {
    /**
     * Nombre del equipo de la red cuya MAC es "hermana" de [bleAddress], o null si no hay una
     * sola coincidencia clara. [networkDevices]: (MAC, nombre). Solo tiene sentido con
     * direcciones Bluetooth públicas, no aleatorias.
     */
    fun siblingName(bleAddress: String, networkDevices: List<Pair<String, String>>, maxDelta: Int = 3): String? {
        val ble = parse(bleAddress) ?: return null
        return networkDevices.mapNotNull { (mac, name) ->
            val wifi = parse(mac) ?: return@mapNotNull null
            if ((0 until 5).all { wifi[it] == ble[it] } && abs(wifi[5] - ble[5]) <= maxDelta) name else null
        }.distinct().singleOrNull()
    }

    private fun parse(mac: String): IntArray? {
        val parts = mac.trim().split(':', '-')
        if (parts.size != 6) return null
        return IntArray(6) { i -> parts[i].toIntOrNull(16)?.takeIf { it in 0..255 } ?: return null }
    }
}
