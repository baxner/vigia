package com.baxner.vigia.data.radar

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Lógica de estados de la señal Wi-Fi, con fakes (booleanos/enteros, sin WifiManager real). Ver WifiSignalMath.kt. */
class WifiSignalMathTest {

    // ---- bssidEquals ----

    @Test
    fun `compara BSSID sin distinguir mayusculas`() {
        assertTrue(bssidEquals("AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff"))
        assertFalse(bssidEquals(null, "aa:bb:cc:dd:ee:ff"))
        assertFalse(bssidEquals("AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"))
    }

    // ---- wifiConnectedState ----

    @Test
    fun `AP conectado sin permiso`() {
        assertEquals(SignalSourceState.NO_PERMISSION, wifiConnectedState(hasPermission = false, radioOn = true, connectedToTarget = true))
    }

    @Test
    fun `AP conectado con wifi apagado`() {
        assertEquals(SignalSourceState.RADIO_OFF, wifiConnectedState(hasPermission = true, radioOn = false, connectedToTarget = true))
    }

    @Test
    fun `el permiso manda sobre el radio apagado`() {
        assertEquals(SignalSourceState.NO_PERMISSION, wifiConnectedState(hasPermission = false, radioOn = false, connectedToTarget = true))
    }

    @Test
    fun `el telefono se desconecto del AP objetivo`() {
        assertEquals(SignalSourceState.NOT_FOUND, wifiConnectedState(hasPermission = true, radioOn = true, connectedToTarget = false))
    }

    @Test
    fun `AP conectado todo bien`() {
        assertEquals(SignalSourceState.OK, wifiConnectedState(hasPermission = true, radioOn = true, connectedToTarget = true))
    }

    // ---- wifiScanState ----

    @Test
    fun `escaneo sin permiso o radio apagado, antes que nada`() {
        assertEquals(
            SignalSourceState.NO_PERMISSION,
            wifiScanState(hasPermission = false, radioOn = true, foundThisScan = true, throttled = false, consecutiveMisses = 0),
        )
        assertEquals(
            SignalSourceState.RADIO_OFF,
            wifiScanState(hasPermission = true, radioOn = false, foundThisScan = true, throttled = false, consecutiveMisses = 0),
        )
    }

    @Test
    fun `encontrado es OK aunque el escaneo haya sido limitado`() {
        assertEquals(
            SignalSourceState.OK,
            wifiScanState(hasPermission = true, radioOn = true, foundThisScan = true, throttled = true, consecutiveMisses = 5),
        )
    }

    @Test
    fun `no encontrado pero limitado es SLOW_THROTTLED, no NOT_FOUND`() {
        assertEquals(
            SignalSourceState.SLOW_THROTTLED,
            wifiScanState(hasPermission = true, radioOn = true, foundThisScan = false, throttled = true, consecutiveMisses = 10),
        )
    }

    @Test
    fun `no encontrado 3 veces seguidas sin estar limitado es NOT_FOUND`() {
        assertEquals(
            SignalSourceState.NOT_FOUND,
            wifiScanState(hasPermission = true, radioOn = true, foundThisScan = false, throttled = false, consecutiveMisses = 3),
        )
    }

    @Test
    fun `menos de 3 fallos seguidos todavia no es NOT_FOUND`() {
        assertEquals(
            SignalSourceState.OK,
            wifiScanState(hasPermission = true, radioOn = true, foundThisScan = false, throttled = false, consecutiveMisses = 2),
        )
    }

    @Test
    fun `el umbral de fallos es configurable`() {
        assertEquals(
            SignalSourceState.NOT_FOUND,
            wifiScanState(hasPermission = true, radioOn = true, foundThisScan = false, throttled = false, consecutiveMisses = 5, missThreshold = 5),
        )
        assertEquals(
            SignalSourceState.OK,
            wifiScanState(hasPermission = true, radioOn = true, foundThisScan = false, throttled = false, consecutiveMisses = 4, missThreshold = 5),
        )
    }
}
