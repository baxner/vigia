package com.baxner.vigia.data.agent

import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.Device
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * La subida única de personalizaciones al vincular (ver el encargo, punto
 * 4): empareja por MAC y, si falta, por IP + nombre fuerte; nunca borra del
 * lado del agente lo que acá no se conoce. Pura — listas en memoria, sin
 * red — ver [AgentCustomizationMigrator].
 */
class AgentCustomizationMigratorTest {
    private val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
    private val now: Instant = Instant.parse("2026-01-01T10:00:00Z")

    private fun known(
        id: Long,
        ip: String,
        mac: String? = null,
        hostname: String = "",
        custom: DeviceCustomization = DeviceCustomization(),
    ) = KnownDevice(
        id = id,
        network = network,
        mac = mac,
        lastIp = ip,
        firstSeen = now,
        lastSeen = now,
        online = true,
        onlineSince = now,
        last = Device(ip = ip, mac = mac.orEmpty(), hostname = hostname),
        custom = custom,
    )

    @Test
    fun `sin ninguna personalizacion local no hay nada que subir`() {
        val local = listOf(known(1, "192.168.0.10", mac = "aa:aa:aa:aa:aa:01"))
        val remote = listOf(known(100, "192.168.0.10", mac = "aa:aa:aa:aa:aa:01"))

        assertTrue(AgentCustomizationMigrator.plan(local, remote).isEmpty())
    }

    @Test
    fun `empareja por MAC y manda solo los campos puestos`() {
        val mac = "aa:aa:aa:aa:aa:01"
        val local = listOf(known(1, "192.168.0.10", mac = mac, custom = DeviceCustomization(name = "Foco de la sala", important = true)))
        val remote = listOf(known(100, "192.168.0.99", mac = mac)) // IP distinta en el agente: solo la MAC debe bastar.

        val plan = AgentCustomizationMigrator.plan(local, remote)

        assertEquals(1, plan.size)
        val (remoteId, patch) = plan.single()
        assertEquals(100L, remoteId)
        val json = patch.toJsonObject()
        assertEquals(setOf("name", "important"), json.keys)
    }

    @Test
    fun `sin MAC de un lado empareja por IP + nombre fuerte`() {
        val local = listOf(known(1, "192.168.0.10", hostname = "foco-sala", custom = DeviceCustomization(location = "Sala")))
        val remote = listOf(known(100, "192.168.0.10", hostname = "foco-sala"))

        val plan = AgentCustomizationMigrator.plan(local, remote)

        assertEquals(1, plan.size)
        assertEquals(100L, plan.single().first)
    }

    @Test
    fun `misma IP pero sin nombre fuerte no empareja (podria ser otro equipo)`() {
        val local = listOf(known(1, "192.168.0.10", custom = DeviceCustomization(location = "Sala")))
        val remote = listOf(known(100, "192.168.0.10"))

        assertTrue(AgentCustomizationMigrator.plan(local, remote).isEmpty())
    }

    @Test
    fun `mismo nombre fuerte pero IP distinta no empareja sin MAC`() {
        val local = listOf(known(1, "192.168.0.10", hostname = "foco-sala", custom = DeviceCustomization(location = "Sala")))
        val remote = listOf(known(100, "192.168.0.11", hostname = "foco-sala"))

        assertTrue(AgentCustomizationMigrator.plan(local, remote).isEmpty())
    }

    @Test
    fun `MAC distinta nunca empareja aunque la IP coincida`() {
        val local = listOf(known(1, "192.168.0.10", mac = "aa:aa:aa:aa:aa:01", custom = DeviceCustomization(location = "Sala")))
        val remote = listOf(known(100, "192.168.0.10", mac = "bb:bb:bb:bb:bb:02"))

        assertTrue(AgentCustomizationMigrator.plan(local, remote).isEmpty())
    }

    @Test
    fun `un mismo remoto no se usa dos veces`() {
        val mac = "aa:aa:aa:aa:aa:01"
        val local = listOf(
            known(1, "192.168.0.10", mac = mac, custom = DeviceCustomization(name = "Uno")),
            known(2, "192.168.0.11", mac = mac, custom = DeviceCustomization(name = "Dos")),
        )
        val remote = listOf(known(100, "192.168.0.10", mac = mac))

        val plan = AgentCustomizationMigrator.plan(local, remote)

        assertEquals(1, plan.size) // solo el primero se queda con el único remoto disponible.
    }

    @Test
    fun `sin equipo remoto que empareje, ese local simplemente no sube nada`() {
        val local = listOf(known(1, "192.168.0.10", mac = "aa:aa:aa:aa:aa:01", custom = DeviceCustomization(name = "Foco")))
        val remote = emptyList<KnownDevice>()

        assertTrue(AgentCustomizationMigrator.plan(local, remote).isEmpty())
    }
}
