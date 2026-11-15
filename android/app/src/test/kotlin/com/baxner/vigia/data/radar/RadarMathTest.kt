package com.baxner.vigia.data.radar

import java.util.Random
import kotlin.math.abs
import kotlin.math.cos
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Matemáticas de "Buscar señal" con datos sintéticos y semilla fija (ver RadarMath.kt). */
class RadarMathTest {

    // ---- Filtro de señal ----

    @Test
    fun `el filtro converge a la señal real a pesar del ruido`() {
        val random = Random(7)
        val filter = RssiFilter.forBle()
        repeat(200) { filter.update(-60.0 + random.nextGaussian() * 4) }
        assertEquals(-60.0, filter.value!!, 1.5)
    }

    @Test
    fun `el filtro sigue un cambio real (acercarse)`() {
        val filter = RssiFilter.forWifi()
        repeat(10) { filter.update(-75.0) }
        repeat(8) { filter.update(-55.0) }
        assertTrue("tras 8 lecturas debía estar cerca de −55, quedó en ${filter.value}", filter.value!! > -60.0)
    }

    // ---- Distancia ----

    @Test
    fun `la distancia sale como rango ordenado y crece al bajar la señal`() {
        val ref = -38.0
        val cerca = DistanceModel.estimate(-45.0, ref)
        val lejos = DistanceModel.estimate(-75.0, ref)
        assertTrue(cerca.minMeters <= cerca.typicalMeters && cerca.typicalMeters <= cerca.maxMeters)
        assertTrue(lejos.typicalMeters > cerca.typicalMeters)
        assertEquals(1.0, DistanceModel.estimate(ref, ref).typicalMeters, 0.01)
    }

    @Test
    fun `muy cerca (señal más fuerte que la referencia) el rango sigue ordenado`() {
        val r = DistanceModel.estimate(-30.0, -41.0)
        assertTrue("min ${r.minMeters} > max ${r.maxMeters}", r.minMeters <= r.maxMeters)
        assertTrue(r.maxMeters < 1.0)
    }

    @Test
    fun `la referencia a 1 m usa la potencia anunciada del Bluetooth`() {
        val conPotencia = SignalTarget.Ble("aa:bb:cc:dd:ee:01", "Tira", "Govee", txPowerDbm = 0, label = "Tira")
        val sinPotencia = conPotencia.copy(txPowerDbm = null)
        assertEquals(-41.0, DistanceModel.referenceAt1m(conPotencia), 0.0)
        assertEquals(-62.0, DistanceModel.referenceAt1m(sinPotencia), 0.0)
    }

    // ---- Tendencia ----

    @Test
    fun `la tendencia detecta acercarse y alejarse, y no salta con el ruido`() {
        val trend = TrendDetector(windowMillis = 3_000, thresholdDb = 2.0)
        var t = 0L
        var last = Trend.STEADY
        for (rssi in listOf(-70.0, -69.0, -67.0, -65.0, -63.0)) {
            last = trend.update(t, rssi)
            t += 1_000
        }
        assertEquals(Trend.CLOSER, last)

        val ruido = TrendDetector()
        t = 0
        var r = Trend.CLOSER
        for (rssi in listOf(-60.0, -60.5, -59.6, -60.3, -59.9, -60.2)) {
            r = ruido.update(t, rssi)
            t += 1_000
        }
        assertEquals(Trend.STEADY, r)
    }

    @Test
    fun `con lecturas espaciadas igual compara contra la de hace una ventana`() {
        val trend = TrendDetector(windowMillis = 8_000)
        trend.update(0, -80.0)
        trend.update(3_000, -78.0)
        assertEquals(Trend.FARTHER, trend.update(9_000, -84.0))
    }

    // ---- Vuelta guiada (rumbo) ----

    /** Una vuelta: [perSector] lecturas por sector de 45°, con sombra del cuerpo de [swingDb] y ruido gaussiano. */
    private fun vuelta(targetDeg: Double, swingDb: Double, noiseDb: Double, perSector: Int, seed: Long): List<AzimuthRssi> {
        val random = Random(seed)
        return (0 until BearingEstimator.Sectors).flatMap { sector ->
            (0 until perSector).map { i ->
                val az = sector * 45.0 + (i + 0.5) * 45.0 / perSector
                val signal = -65.0 + (swingDb / 2) * cos(Math.toRadians(az - targetDeg)) + random.nextGaussian() * noiseDb
                AzimuthRssi(az, signal)
            }
        }
    }

    private fun angleError(a: Double, b: Double): Double {
        val d = abs(a - b) % 360
        return if (d > 180) 360 - d else d
    }

    @Test
    fun `una vuelta con sombra del cuerpo apunta al objetivo con menos de 30 grados de error`() {
        for ((target, seed) in listOf(120.0 to 1L, 10.0 to 2L, 275.0 to 3L)) {
            val estimate = BearingEstimator.estimate(vuelta(target, swingDb = 8.0, noiseDb = 4.0, perSector = 5, seed = seed))
            assertNotNull(estimate)
            assertTrue("objetivo $target: dio ${estimate!!.bearingDeg}", angleError(estimate.bearingDeg, target) < 30)
            assertNotEquals(BearingConfidence.INCONCLUSIVE, estimate.confidence)
        }
    }

    @Test
    fun `cerca del norte no se confunde por el cruce de 360 a 0`() {
        val estimate = BearingEstimator.estimate(vuelta(350.0, swingDb = 10.0, noiseDb = 3.0, perSector = 6, seed = 9))
        assertTrue("dio ${estimate!!.bearingDeg}", angleError(estimate.bearingDeg, 350.0) < 30)
    }

    @Test
    fun `sin sombra del cuerpo dice no concluyente`() {
        val estimate = BearingEstimator.estimate(vuelta(0.0, swingDb = 0.0, noiseDb = 4.0, perSector = 5, seed = 4))
        assertEquals(BearingConfidence.INCONCLUSIVE, estimate!!.confidence)
    }

    @Test
    fun `sin la vuelta completa todavía no hay rumbo`() {
        val incompleta = vuelta(90.0, swingDb = 8.0, noiseDb = 2.0, perSector = 3, seed = 5)
            .filter { BearingEstimator.sectorOf(it.azimuthDeg) != 6 }
        assertNull(BearingEstimator.estimate(incompleta))
        assertEquals(0, BearingEstimator.sectorCounts(incompleta)[6])
    }

    @Test
    fun `quedarse mucho rato mirando hacia un lado no tuerce el rumbo`() {
        val base = vuelta(200.0, swingDb = 8.0, noiseDb = 3.0, perSector = 3, seed = 6)
        // 40 lecturas de más hacia el este (sector 2), todas con la señal del ajuste en ese rumbo.
        val extra = (0 until 40).map { AzimuthRssi(100.0, -65.0 + 4 * cos(Math.toRadians(100.0 - 200.0))) }
        val estimate = BearingEstimator.estimate(base + extra)
        assertTrue("dio ${estimate!!.bearingDeg}", angleError(estimate.bearingDeg, 200.0) < 30)
    }

    // ---- Geiger ----

    @Test
    fun `el Geiger acelera con la señal y respeta sus límites`() {
        val debil = Geiger.intervalMillis(-100.0)
        val media = Geiger.intervalMillis(-65.0)
        val fuerte = Geiger.intervalMillis(-30.0)
        assertEquals(Geiger.SlowestMillis, debil)
        assertEquals(Geiger.FastestMillis, fuerte)
        assertTrue(media in (fuerte + 1) until debil)
    }

    // ---- Parentesco de MAC ----

    @Test
    fun `reconoce la MAC hermana de un equipo IoT`() {
        val red = listOf("02:00:00:12:34:10" to "Tira Govee", "02:00:00:af:91:42" to "TV")
        assertEquals("Tira Govee", MacKinship.siblingName("02:00:00:12:34:12", red))
        assertNull(MacKinship.siblingName("02:00:00:12:35:12", red))
        assertNull(MacKinship.siblingName("no-es-mac", red))
    }

    @Test
    fun `si dos equipos podrían ser, no adivina`() {
        val red = listOf("02:00:00:12:34:10" to "Foco 1", "02:00:00:12:34:13" to "Foco 2")
        assertNull(MacKinship.siblingName("02:00:00:12:34:11", red))
    }
}
