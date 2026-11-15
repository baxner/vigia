package com.baxner.vigia.data.history

import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.ScanResult
import java.time.Instant
import java.time.temporal.ChronoUnit
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * [RoomHistoryRepository] con una base en memoria vía Robolectric: la
 * reconciliación y la regla de gracia ya están probadas aparte sin Room
 * ([DeviceReconcilerTest], [DeviceHistoryEngineTest]); esto verifica que la
 * conexión con Room — DAOs, transacciones, serialización a JSON y lectura de
 * vuelta — funciona de punta a punta.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class RoomHistoryRepositoryTest {

    private lateinit var db: HistoryDatabase
    private lateinit var repository: RoomHistoryRepository

    private val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
    private val t0: Instant = Instant.parse("2026-01-01T10:00:00Z")

    @Before
    fun setUp() {
        db = Room.inMemoryDatabaseBuilder(ApplicationProvider.getApplicationContext(), HistoryDatabase::class.java)
            .allowMainThreadQueries()
            .build()
        repository = RoomHistoryRepository(db)
    }

    @After
    fun tearDown() {
        db.close()
    }

    private fun device(ip: String, mac: String = "", hostname: String = "", extra: Map<String, String> = emptyMap()) =
        Device(ip = ip, mac = mac, hostname = hostname, extra = extra)

    private fun scanOf(vararg devices: Device) = ScanResult(subnet = "192.168.0.0/24", devices = devices.toList())

    @Test
    fun `un escaneo sin conocidos crea el equipo y dispara FIRST_SEEN`() = runTest {
        val escaneado = device(ip = "192.168.0.10", mac = "aa:bb:cc:dd:ee:01")

        val events = repository.recordScan(network, scanOf(escaneado), t0)

        assertEquals(listOf(DeviceEventType.FIRST_SEEN), events.map { it.type })
        val known = repository.observeDevices(network).first()
        assertEquals(1, known.size)
        assertEquals("192.168.0.10", known.single().lastIp)
        assertEquals("aa:bb:cc:dd:ee:01", known.single().mac)
        assertTrue(known.single().online)
    }

    @Test
    fun `un segundo escaneo idéntico no genera eventos`() = runTest {
        val escaneado = device(ip = "192.168.0.10", mac = "aa:bb:cc:dd:ee:01")
        repository.recordScan(network, scanOf(escaneado), t0)

        val events = repository.recordScan(network, scanOf(escaneado), t0.plusSeconds(30))

        assertTrue(events.isEmpty())
        assertEquals(1, repository.observeDevices(network).first().size)
    }

    @Test
    fun `cambia de IP y se reconoce por MAC con IP_CHANGED`() = runTest {
        val mac = "aa:bb:cc:dd:ee:01"
        repository.recordScan(network, scanOf(device(ip = "192.168.0.10", mac = mac)), t0)

        val events = repository.recordScan(network, scanOf(device(ip = "192.168.0.99", mac = mac)), t0.plusSeconds(30))

        assertEquals(listOf(DeviceEventType.IP_CHANGED), events.map { it.type })
        assertEquals("192.168.0.10 → 192.168.0.99", events.single().detail)
        val known = repository.observeDevices(network).first().single()
        assertEquals("192.168.0.99", known.lastIp)
        val id = known.id
        // observeEvents trae toda la cronología, más reciente primero: IP_CHANGED (escaneo 2) y,
        // antes, el FIRST_SEEN del primer escaneo.
        assertEquals(
            listOf(DeviceEventType.IP_CHANGED, DeviceEventType.FIRST_SEEN),
            repository.observeEvents(id).first().map { it.type },
        )
    }

    @Test
    fun `la regla de gracia persiste entre escaneos- falta dos veces y vuelve`() = runTest {
        val mac = "aa:bb:cc:dd:ee:01"
        repository.recordScan(network, scanOf(device(ip = "192.168.0.10", mac = mac)), t0)

        // Falta 1: sin el equipo en el escaneo de su red. Sin evento todavía.
        val falta1 = repository.recordScan(network, scanOf(), t0.plusSeconds(60))
        assertTrue(falta1.isEmpty())
        var known = repository.observeDevices(network).first().single()
        assertEquals(false, known.online)
        assertEquals(t0, known.onlineSince)

        // Falta 2: ahora sí se compromete el WENT_OFFLINE, fechado con el último lastSeen
        // real (t0), no con t0+120s (el momento en que recién lo confirmamos).
        val falta2 = repository.recordScan(network, scanOf(), t0.plusSeconds(120))
        assertEquals(listOf(DeviceEventType.WENT_OFFLINE), falta2.map { it.type })
        assertEquals(t0, falta2.single().at)
        known = repository.observeDevices(network).first().single()
        assertNull(known.onlineSince)

        // Vuelve: CAME_ONLINE con onlineSince = ahora.
        val ahora = t0.plusSeconds(600)
        val vuelve = repository.recordScan(network, scanOf(device(ip = "192.168.0.10", mac = mac)), ahora)
        assertEquals(listOf(DeviceEventType.CAME_ONLINE), vuelve.map { it.type })
        known = repository.observeDevices(network).first().single()
        assertTrue(known.online)
        assertEquals(ahora, known.onlineSince)

        // La cronología completa quedó grabada en orden (más reciente primero).
        val historial = repository.observeEvents(known.id).first().map { it.type }
        assertEquals(listOf(DeviceEventType.CAME_ONLINE, DeviceEventType.WENT_OFFLINE, DeviceEventType.FIRST_SEEN), historial)
    }

    @Test
    fun `mas de diez minutos sin verlo compromete WENT_OFFLINE aunque sea la primera falta`() = runTest {
        val mac = "aa:bb:cc:dd:ee:01"
        repository.recordScan(network, scanOf(device(ip = "192.168.0.10", mac = mac)), t0)

        val events = repository.recordScan(network, scanOf(), t0.plus(11, ChronoUnit.MINUTES))

        assertEquals(listOf(DeviceEventType.WENT_OFFLINE), events.map { it.type })
    }

    @Test
    fun `observeDevices ordena por IP numerica, no como texto`() = runTest {
        // Como texto, "105" y "29" quedarían antes de "9". Numérico: 9 antes de 29 antes de 105.
        repository.recordScan(
            network,
            scanOf(
                device(ip = "192.168.0.105", mac = "aa:aa:aa:aa:aa:01"),
                device(ip = "192.168.0.9", mac = "aa:aa:aa:aa:aa:02"),
                device(ip = "192.168.0.29", mac = "aa:aa:aa:aa:aa:03"),
            ),
            t0,
        )

        val ips = repository.observeDevices(network).first().map { it.lastIp }

        assertEquals(listOf("192.168.0.9", "192.168.0.29", "192.168.0.105"), ips)
    }

    @Test
    fun `updateCustomization persiste y se puede leer de vuelta`() = runTest {
        repository.recordScan(network, scanOf(device(ip = "192.168.0.10", mac = "aa:bb:cc:dd:ee:01")), t0)
        val id = repository.observeDevices(network).first().single().id

        repository.updateCustomization(id, DeviceCustomization(name = "Laptop de prueba", important = true))

        val actualizado = repository.observeDevice(id).first()
        assertEquals("Laptop de prueba", actualizado?.custom?.name)
        assertEquals(true, actualizado?.custom?.important)
    }

    @Test
    fun `forget borra el equipo y sus eventos`() = runTest {
        repository.recordScan(network, scanOf(device(ip = "192.168.0.10", mac = "aa:bb:cc:dd:ee:01")), t0)
        val id = repository.observeDevices(network).first().single().id

        repository.forget(id)

        assertTrue(repository.observeDevices(network).first().isEmpty())
        assertNull(repository.observeDevice(id).first())
        assertTrue(repository.observeEvents(id).first().isEmpty())
    }
}
