package com.baxner.vigia.data.radar

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Lógica pura de Bluetooth LE con datos sintéticos (ver BleMath.kt). */
class BleMathTest {

    // ---- Heurística de dirección aleatoria ----

    @Test
    fun `direccion estatica (bits altos 11) se marca aleatoria`() {
        assertTrue(looksLikeRandomAddress("C0:11:22:33:44:55"))
        assertTrue(looksLikeRandomAddress("FF:11:22:33:44:55"))
    }

    @Test
    fun `direccion privada resoluble (bits altos 01) se marca aleatoria`() {
        assertTrue(looksLikeRandomAddress("40:11:22:33:44:55"))
        assertTrue(looksLikeRandomAddress("7F:11:22:33:44:55"))
    }

    @Test
    fun `patrones ambiguos no se marcan aleatorios (sin mentir)`() {
        assertFalse(looksLikeRandomAddress("00:11:22:33:44:55"))
        assertFalse(looksLikeRandomAddress("80:11:22:33:44:55"))
    }

    @Test
    fun `direccion invalida no rompe nada`() {
        assertFalse(looksLikeRandomAddress(""))
        assertFalse(looksLikeRandomAddress("no-es-mac"))
    }

    // ---- Agregación de anuncios BLE ----

    private fun device(address: String, lastSeen: Long, rssi: Int = -60) =
        BleDevice(address, name = null, brand = null, rssi = rssi, txPowerDbm = null, lastSeenMillis = lastSeen, randomAddress = false)

    @Test
    fun `un anuncio nuevo se agrega y la lista queda ordenada por lo mas reciente`() {
        var devices = aggregateBleDevices(emptyList(), device("AA:00:00:00:00:01", lastSeen = 1_000))
        devices = aggregateBleDevices(devices, device("AA:00:00:00:00:02", lastSeen = 2_000))
        devices = aggregateBleDevices(devices, device("AA:00:00:00:00:03", lastSeen = 1_500))

        assertEquals(listOf("AA:00:00:00:00:02", "AA:00:00:00:00:03", "AA:00:00:00:00:01"), devices.map { it.address })
    }

    @Test
    fun `un anuncio de una direccion ya vista la reemplaza, no la duplica`() {
        var devices = aggregateBleDevices(emptyList(), device("AA:00:00:00:00:01", lastSeen = 1_000, rssi = -80))
        devices = aggregateBleDevices(devices, device("AA:00:00:00:00:02", lastSeen = 1_100))
        devices = aggregateBleDevices(devices, device("AA:00:00:00:00:01", lastSeen = 2_000, rssi = -40))

        assertEquals(2, devices.size)
        val updated = devices.first { it.address == "AA:00:00:00:00:01" }
        assertEquals(-40, updated.rssi)
        assertEquals(2_000L, updated.lastSeenMillis)
    }

    // ---- Estados (con fakes: solo booleanos, sin BluetoothAdapter real) ----

    @Test
    fun `sin permiso manda sobre todo lo demas`() {
        assertEquals(SignalSourceState.NO_PERMISSION, bleScanState(hasPermission = false, radioOn = false))
        assertEquals(SignalSourceState.NO_PERMISSION, bleScanState(hasPermission = false, radioOn = true))
    }

    @Test
    fun `con permiso pero radio apagado`() {
        assertEquals(SignalSourceState.RADIO_OFF, bleScanState(hasPermission = true, radioOn = false))
    }

    @Test
    fun `con permiso y radio encendido esta OK`() {
        assertEquals(SignalSourceState.OK, bleScanState(hasPermission = true, radioOn = true))
    }
}
