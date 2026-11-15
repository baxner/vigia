package com.baxner.vigia.ui.devices

import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.Device
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

class IsNewTest {
    private val t0 = Instant.parse("2026-10-07T04:57:00Z") // primer escaneo de la red

    private fun known(firstSeen: Instant) = KnownDevice(
        id = 1, network = NetworkKey("k"), mac = null, lastIp = "192.168.0.9",
        firstSeen = firstSeen, lastSeen = firstSeen, online = true, onlineSince = firstSeen, last = Device(ip = "192.168.0.9"),
    )

    @Test
    fun `lo del primer escaneo es linea base, no Nuevo`() {
        val now = t0.plusSeconds(3600)
        assertFalse(known(t0).isNew(now, baseline = t0))
        assertFalse(known(t0.plusSeconds(5 * 60)).isNew(now, baseline = t0)) // dentro del margen
    }

    @Test
    fun `un equipo que llega despues de la linea base es Nuevo por 24 h`() {
        val llegada = t0.plusSeconds(3 * 3600)
        assertTrue(known(llegada).isNew(llegada.plusSeconds(60), baseline = t0))
        assertFalse(known(llegada).isNew(llegada.plusSeconds(25 * 3600), baseline = t0))
    }

    @Test
    fun `baselineOf es el firstSeen mas antiguo`() {
        assertEquals(t0, baselineOf(listOf(known(t0.plusSeconds(99)), known(t0))))
        assertEquals(null, baselineOf(emptyList()))
    }
}
