package com.baxner.vigia.ui.radar

import com.baxner.vigia.data.radar.BearingConfidence
import com.baxner.vigia.data.radar.FakeHeadingSource
import com.baxner.vigia.data.radar.FakeSignalSource
import com.baxner.vigia.data.radar.HeadingSample
import com.baxner.vigia.data.radar.SignalSample
import com.baxner.vigia.data.radar.SignalTarget
import com.baxner.vigia.data.radar.Trend
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.math.cos
import kotlin.math.roundToInt
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * [HuntViewModel] con [FakeSignalSource]/[FakeHeadingSource] y reloj fijo (el
 * encargo, "Pruebas": "reloj fijo en las pruebas"): cada [SignalSample]/
 * [HeadingSample] lleva su propio `atMillis` explícito, nunca la hora real.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class HuntViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private val wifiTarget = SignalTarget.WifiAp(
        bssid = "02:00:00:00:00:01",
        ssid = "Router-Lab",
        frequencyMhz = 5180,
        rttCapable = false,
        connected = false,
        label = "Router-Lab",
    )

    private val bleTarget = SignalTarget.Ble(
        address = "02:00:00:00:00:10",
        name = "Tira",
        brand = "Govee",
        txPowerDbm = 0,
        label = "Tira",
    )

    @Test
    fun `suavizado, tendencia y distancia`() = runTest(dispatcher) {
        val signal = FakeSignalSource()
        val heading = FakeHeadingSource()
        val vm = HuntViewModel(signal, heading)
        vm.start(wifiTarget)
        advanceUntilIdle()

        var t = 0L
        for (rssi in listOf(-80, -80, -78, -70, -60, -55)) {
            signal.emit(SignalSample(atMillis = t, rssi = rssi))
            advanceUntilIdle()
            t += 2_000
        }

        val state = vm.uiState.value
        assertNotNull("debía haber una señal suavizada", state.smoothedRssi)
        assertEquals("debía detectar que se acerca, dio ${state.trend}", Trend.CLOSER, state.trend)
        assertNotNull("debía poder estimar una distancia", state.distance)
        assertTrue(state.distance!!.minMeters <= state.distance!!.typicalMeters)
        assertTrue(state.distance!!.typicalMeters <= state.distance!!.maxMeters)
        vm.stop()
    }

    @Test
    fun `lecturas con RTT`() = runTest(dispatcher) {
        val signal = FakeSignalSource()
        val heading = FakeHeadingSource()
        val vm = HuntViewModel(signal, heading)
        vm.start(wifiTarget)
        advanceUntilIdle()

        signal.emit(SignalSample(atMillis = 0, rssi = -50, rttMeters = 1.8, rttStdDevMeters = 0.2))
        advanceUntilIdle()

        val state = vm.uiState.value
        assertNotNull("debía guardar la medición de RTT", state.rttMeters)
        assertEquals(1.8, state.rttMeters!!, 0.001)
        // El rango por señal sigue disponible aunque haya RTT (por si se pierde la medición exacta).
        assertNotNull(state.distance)
        vm.stop()
    }

    /** Una vuelta completa: [perSector] pares rumbo/señal por cada uno de los 8 sectores, con sombra de [swingDb] hacia [targetDeg]. */
    private suspend fun TestScope.completeGuidedTurn(
        vm: HuntViewModel,
        signal: FakeSignalSource,
        heading: FakeHeadingSource,
        targetDeg: Double,
        swingDb: Double,
        perSector: Int,
    ) {
        vm.startGuidedTurn()
        var t = 0L
        for (sector in 0 until 8) {
            repeat(perSector) { i ->
                val az = sector * 45.0 + (i + 0.5) * 45.0 / perSector
                val rssi = -65.0 + (swingDb / 2) * cos(Math.toRadians(az - targetDeg))
                heading.emit(HeadingSample(atMillis = t, azimuthDeg = az.toFloat(), reliable = true))
                advanceUntilIdle()
                signal.emit(SignalSample(atMillis = t, rssi = rssi.roundToInt()))
                advanceUntilIdle()
                t += 200
            }
        }
    }

    @Test
    fun `la vuelta guiada junta pares y entrega el rumbo al completarse`() = runTest(dispatcher) {
        val signal = FakeSignalSource()
        val heading = FakeHeadingSource()
        val vm = HuntViewModel(signal, heading)
        vm.start(bleTarget)
        advanceUntilIdle()

        // La vuelta usa la señal cruda (no la suavizada): basta con las 5 lecturas por sector.
        completeGuidedTurn(vm, signal, heading, targetDeg = 120.0, swingDb = 10.0, perSector = 5)

        val state = vm.uiState.value
        assertNotNull("debía tener un rumbo tras la vuelta completa", state.bearing)
        assertFalse("la vuelta debía marcarse como terminada", state.guidedTurnActive)
        assertTrue("el anillo debía llenarse en los 8 sectores", state.sectorCounts.all { it >= 5 })
        assertEquals(120.0, state.bearing!!.bearingDeg, 20.0)
        vm.stop()
    }

    @Test
    fun `sin sombra del cuerpo dice no concluyente`() = runTest(dispatcher) {
        val signal = FakeSignalSource()
        val heading = FakeHeadingSource()
        val vm = HuntViewModel(signal, heading)
        vm.start(bleTarget)
        advanceUntilIdle()

        completeGuidedTurn(vm, signal, heading, targetDeg = 0.0, swingDb = 0.0, perSector = 5)

        val bearing = vm.uiState.value.bearing
        assertNotNull("con lecturas completas debía haber una estimación, aunque sea no concluyente", bearing)
        assertEquals(BearingConfidence.INCONCLUSIVE, bearing!!.confidence)
        vm.stop()
    }

    @Test
    fun `cambiar de objetivo reinicia el estado, reanudar el mismo lo conserva`() = runTest(dispatcher) {
        val signal = FakeSignalSource()
        val heading = FakeHeadingSource()
        val vm = HuntViewModel(signal, heading)
        vm.start(wifiTarget)
        advanceUntilIdle()
        signal.emit(SignalSample(atMillis = 0, rssi = -60))
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.smoothedRssi)

        // Pausa (pantalla no visible) y reanuda con el MISMO objetivo: conserva el estado.
        vm.stop()
        vm.start(wifiTarget)
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.smoothedRssi)

        // Un objetivo DISTINTO sí reinicia.
        vm.start(bleTarget)
        advanceUntilIdle()
        assertEquals(null, vm.uiState.value.smoothedRssi)
        vm.stop()
    }

    @Test
    fun `el giro relativo va de -180 a 180 y a la derecha es positivo`() {
        assertEquals(60.0, relativeTurnDegrees(bearingDeg = 120.0, headingDeg = 60.0), 0.001)
        assertEquals(-30.0, relativeTurnDegrees(bearingDeg = 350.0, headingDeg = 20.0), 0.001)
        assertEquals(180.0, kotlin.math.abs(relativeTurnDegrees(bearingDeg = 0.0, headingDeg = 180.0)), 0.001)
    }

    @Test
    fun `onCleared cancela el scope y ya no procesa nuevas muestras`() = runTest(dispatcher) {
        val signal = FakeSignalSource()
        val heading = FakeHeadingSource()
        val vm = HuntViewModel(signal, heading)
        vm.start(wifiTarget)
        advanceUntilIdle()
        signal.emit(SignalSample(atMillis = 0, rssi = -60))
        advanceUntilIdle()
        assertEquals(-60.0, vm.uiState.value.smoothedRssi ?: 0.0, 0.1)

        vm.onCleared()
        signal.emit(SignalSample(atMillis = 1_000, rssi = -30))
        advanceUntilIdle()

        assertEquals(-60.0, vm.uiState.value.smoothedRssi ?: 0.0, 0.1)
    }
}
