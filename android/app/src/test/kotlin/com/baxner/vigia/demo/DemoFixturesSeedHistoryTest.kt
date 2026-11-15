package com.baxner.vigia.demo

import com.baxner.vigia.data.history.DeviceEventType
import com.baxner.vigia.data.history.InMemoryHistoryRepository
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.ui.devices.isNew
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * [DemoFixtures.seedHistory] no es solo un snapshot: prueba que de verdad
 * deja equipos sin conexión, uno "nuevo", un cambio de IP en la cronología
 * y una personalización de ejemplo (el caso "Foco de la sala" del encargo).
 */
class DemoFixturesSeedHistoryTest {
    private val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")

    @Test
    fun `seedHistory deja equipos sin conexion, uno nuevo y uno personalizado`() = runTest {
        val repo = InMemoryHistoryRepository()
        DemoFixtures.seedHistory(repo, network)

        val devices = repo.observeDevices(network).first()
        assertEquals(13, devices.size)

        val offline = devices.filterNot { it.online }
        assertTrue("la impresora vieja debe quedar sin conexión", offline.any { it.lastIp == "192.168.0.15" })

        val pc = devices.first { it.lastIp == "192.168.0.4" }
        assertTrue(pc.online)
        val pcEvents = repo.observeEvents(pc.id).first()
        assertTrue("el PC debe tener un IP_CHANGED en su cronología", pcEvents.any { it.type == DeviceEventType.IP_CHANGED })

        val echo = devices.first { it.lastIp == "192.168.0.52" }
        assertTrue("el Echo Show debe verse como 'Nuevo' (<24h)", echo.isNew())

        val foco = devices.first { it.lastIp == "192.168.0.76" }
        assertEquals("Foco de la sala", foco.custom.name)
        assertEquals("smart_bulb", foco.custom.typeId)
        assertEquals("Sala", foco.custom.location)
        assertTrue(foco.custom.important)
    }
}
