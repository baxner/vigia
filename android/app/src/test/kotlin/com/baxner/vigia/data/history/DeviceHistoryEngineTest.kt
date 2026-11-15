package com.baxner.vigia.data.history

import com.baxner.vigia.data.model.Device
import java.time.Instant
import java.time.temporal.ChronoUnit
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Fusión de estado + eventos (incluida la regla de gracia de WENT_OFFLINE): pura, sin Room. */
class DeviceHistoryEngineTest {

    private val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
    private val t0: Instant = Instant.parse("2026-01-01T10:00:00Z")

    private fun device(ip: String) = Device(ip = ip, mac = "aa:bb:cc:dd:ee:01")

    private fun knownOnline(ip: String = "192.168.0.10", at: Instant = t0) = KnownDevice(
        id = 1,
        network = network,
        mac = "aa:bb:cc:dd:ee:01",
        lastIp = ip,
        firstSeen = at,
        lastSeen = at,
        online = true,
        onlineSince = at,
        last = device(ip),
    )

    // --- Regla de gracia ---

    @Test
    fun `falta una vez sin pasar el umbral no genera evento y conserva onlineSince`() {
        val previo = knownOnline(at = t0)
        val outcome = DeviceHistoryEngine.missing(previo, offlineMissesBefore = 0, at = t0.plusSeconds(60))

        assertTrue(outcome.events.isEmpty())
        assertEquals(1, outcome.offlineMisses)
        assertEquals(false, outcome.device.online)
        assertEquals(t0, outcome.device.onlineSince)
    }

    @Test
    fun `faltar dos escaneos seguidos compromete WENT_OFFLINE`() {
        val previo = knownOnline(at = t0)
        val falta1 = DeviceHistoryEngine.missing(previo, offlineMissesBefore = 0, at = t0.plusSeconds(60))
        val falta2 = DeviceHistoryEngine.missing(falta1.device, offlineMissesBefore = falta1.offlineMisses, at = t0.plusSeconds(120))

        assertEquals(listOf(DeviceEventType.WENT_OFFLINE), falta2.events.map { it.type })
        assertEquals(2, falta2.offlineMisses)
        assertEquals(false, falta2.device.online)
        assertNull(falta2.device.onlineSince)
        // Se fecha con el último lastSeen real (t0), no con el momento en que recién se
        // confirma (t0+120s): es cuando de verdad se fue, no cuando nos enteramos.
        assertEquals(t0, falta2.events.single().at)
    }

    @Test
    fun `mas de diez minutos sin verlo compromete WENT_OFFLINE aunque sea la primera falta`() {
        val previo = knownOnline(at = t0)
        val outcome = DeviceHistoryEngine.missing(previo, offlineMissesBefore = 0, at = t0.plus(11, ChronoUnit.MINUTES))

        assertEquals(1, outcome.offlineMisses)
        assertEquals(listOf(DeviceEventType.WENT_OFFLINE), outcome.events.map { it.type })
        assertNull(outcome.device.onlineSince)
        assertEquals(t0, outcome.events.single().at)
    }

    @Test
    fun `no repite WENT_OFFLINE en escaneos siguientes ya comprometido`() {
        val previo = knownOnline(at = t0)
        val falta1 = DeviceHistoryEngine.missing(previo, offlineMissesBefore = 0, at = t0.plusSeconds(60))
        val falta2 = DeviceHistoryEngine.missing(falta1.device, offlineMissesBefore = falta1.offlineMisses, at = t0.plusSeconds(120))
        val falta3 = DeviceHistoryEngine.missing(falta2.device, offlineMissesBefore = falta2.offlineMisses, at = t0.plusSeconds(180))

        assertTrue(falta3.events.isEmpty())
        assertEquals(3, falta3.offlineMisses)
        assertNull(falta3.device.onlineSince)
    }

    @Test
    fun `si reaparece antes de cumplir la gracia no hay evento y onlineSince se conserva`() {
        val previo = knownOnline(at = t0)
        val falta = DeviceHistoryEngine.missing(previo, offlineMissesBefore = 0, at = t0.plusSeconds(60))

        val vuelve = DeviceHistoryEngine.seen(falta.device, device("192.168.0.10"), at = t0.plusSeconds(90))

        assertTrue("no debe haber ni WENT_OFFLINE ni CAME_ONLINE", vuelve.events.isEmpty())
        assertEquals(true, vuelve.device.online)
        assertEquals(t0, vuelve.device.onlineSince)
        assertEquals(0, vuelve.offlineMisses)
    }

    @Test
    fun `al reaparecer tras un WENT_OFFLINE comprometido dispara CAME_ONLINE con onlineSince ahora`() {
        val previo = knownOnline(at = t0)
        val falta1 = DeviceHistoryEngine.missing(previo, offlineMissesBefore = 0, at = t0.plusSeconds(60))
        val falta2 = DeviceHistoryEngine.missing(falta1.device, offlineMissesBefore = falta1.offlineMisses, at = t0.plusSeconds(120))

        val ahora = t0.plusSeconds(500)
        val vuelve = DeviceHistoryEngine.seen(falta2.device, device("192.168.0.10"), at = ahora)

        assertEquals(listOf(DeviceEventType.CAME_ONLINE), vuelve.events.map { it.type })
        assertEquals(true, vuelve.device.online)
        assertEquals(ahora, vuelve.device.onlineSince)
        assertEquals(0, vuelve.offlineMisses)
    }

    // --- IP_CHANGED ---

    @Test
    fun `ip distinta en un equipo que seguia conectado genera IP_CHANGED`() {
        val previo = knownOnline(ip = "192.168.0.10", at = t0)
        val outcome = DeviceHistoryEngine.seen(previo, device("192.168.0.99"), at = t0.plusSeconds(30))

        val evento = outcome.events.single()
        assertEquals(DeviceEventType.IP_CHANGED, evento.type)
        assertEquals("192.168.0.10 → 192.168.0.99", evento.detail)
        assertEquals("192.168.0.99", outcome.device.lastIp)
        // Sigue conectado sin interrupción: la IP no toca onlineSince.
        assertEquals(t0, outcome.device.onlineSince)
    }

    @Test
    fun `misma ip no genera IP_CHANGED`() {
        val previo = knownOnline(ip = "192.168.0.10", at = t0)
        val outcome = DeviceHistoryEngine.seen(previo, device("192.168.0.10"), at = t0.plusSeconds(30))

        assertTrue(outcome.events.isEmpty())
    }

    @Test
    fun `reaparecer con IP distinta genera CAME_ONLINE e IP_CHANGED en ese orden`() {
        val previo = knownOnline(ip = "192.168.0.10", at = t0)
        val falta1 = DeviceHistoryEngine.missing(previo, offlineMissesBefore = 0, at = t0.plusSeconds(60))
        val falta2 = DeviceHistoryEngine.missing(falta1.device, offlineMissesBefore = falta1.offlineMisses, at = t0.plusSeconds(120))

        val vuelve = DeviceHistoryEngine.seen(falta2.device, device("192.168.0.77"), at = t0.plusSeconds(500))

        assertEquals(listOf(DeviceEventType.CAME_ONLINE, DeviceEventType.IP_CHANGED), vuelve.events.map { it.type })
    }
}
