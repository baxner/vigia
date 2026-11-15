package com.baxner.vigia.data.security

import com.baxner.vigia.data.agent.AlertDto
import com.baxner.vigia.data.agent.OuterRing
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.Identity
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Armado del [SecurityInput] (ver el encargo, "Pruebas"): pura, sin red ni
 * Android. Lo que más importa es qué [SecurityInputDevice.name] elige para
 * cada equipo — es lo que el motor usa para redactar sus textos en español
 * (ver el contrato § "Seguridad" → "Informe": "PC-Escritorio (...)").
 */
class SecurityInputBuilderTest {
    private val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
    private val now = Instant.parse("2026-10-07T20:00:00Z")

    private fun device(
        ip: String = "192.168.0.10",
        hostname: String = "",
        brand: String = "",
        vendor: String = "",
    ) = Device(ip = ip, vendor = vendor, hostname = hostname, identity = Identity(brand = brand))

    private fun knownDevice(
        id: Long = 1L,
        custom: DeviceCustomization = DeviceCustomization(),
        last: Device = device(),
        online: Boolean = true,
    ) = KnownDevice(id = id, network = network, mac = last.mac.ifEmpty { null }, lastIp = last.ip, firstSeen = now, lastSeen = now, online = online, onlineSince = now, last = last, custom = custom)

    @Test
    fun `usa el nombre personalizado si existe, antes que cualquier pista del equipo`() {
        val known = knownDevice(custom = DeviceCustomization(name = "PC del Estudio"), last = device(hostname = "PC-Escritorio"))
        val input = SecurityInputBuilder.build(listOf(known))
        assertEquals("PC del Estudio", input.devices.single().name)
    }

    @Test
    fun `sin nombre personalizado usa el hostname`() {
        val known = knownDevice(last = device(hostname = "PC-Escritorio", vendor = "Gigabyte"))
        val input = SecurityInputBuilder.build(listOf(known))
        assertEquals("PC-Escritorio", input.devices.single().name)
    }

    @Test
    fun `sin hostname usa la marca de la identidad`() {
        val known = knownDevice(last = device(brand = "TP-Link", vendor = "TP-Link Corporation"))
        val input = SecurityInputBuilder.build(listOf(known))
        assertEquals("TP-Link", input.devices.single().name)
    }

    @Test
    fun `sin hostname ni marca usa el fabricante`() {
        val known = knownDevice(last = device(vendor = "Hangzhou Hikvision"))
        val input = SecurityInputBuilder.build(listOf(known))
        assertEquals("Hangzhou Hikvision", input.devices.single().name)
    }

    @Test
    fun `sin ninguna pista usa la IP`() {
        val known = knownDevice(last = device(ip = "192.168.0.67"))
        val input = SecurityInputBuilder.build(listOf(known))
        assertEquals("192.168.0.67", input.devices.single().name)
    }

    @Test
    fun `conserva id, online y el Device tal cual`() {
        val d = device(ip = "192.168.0.41")
        val known = knownDevice(id = 9L, last = d, online = false)
        val input = SecurityInputBuilder.build(listOf(known))
        val result = input.devices.single()
        assertEquals(9L, result.id)
        assertFalse(result.online)
        assertSame(d, result.device)
    }

    @Test
    fun `router, outerRing, wifi y recentAlerts pasan tal cual`() {
        val router = RouterInfo(ip = "192.168.0.4", upnp = true)
        val outerRing = OuterRing(network = "k", subnet = "192.168.100.0/24", gateway = "192.168.100.1")
        val wifi = WifiInfo(ssid = "Casa", security = "wpa2", wps = true)
        val alerts = listOf(AlertDto(id = 1, type = "ROUTER_CHANGED", severity = "high", at = now.toString(), title = "t", body = "b"))

        val input = SecurityInputBuilder.build(devices = emptyList(), router = router, outerRing = outerRing, wifi = wifi, recentAlerts = alerts)

        assertSame(router, input.router)
        assertSame(outerRing, input.outerRing)
        assertSame(wifi, input.wifi)
        assertEquals(alerts, input.recentAlerts)
    }

    @Test
    fun `sin argumentos opcionales, todo queda vacio o null`() {
        val input = SecurityInputBuilder.build(devices = emptyList())
        assertTrue(input.devices.isEmpty())
        assertNull(input.router)
        assertNull(input.outerRing)
        assertNull(input.wifi)
        assertTrue(input.recentAlerts.isEmpty())
    }

    @Test
    fun `varios equipos se mapean en el mismo orden`() {
        val a = knownDevice(id = 1L, last = device(ip = "192.168.0.1"))
        val b = knownDevice(id = 2L, last = device(ip = "192.168.0.2"))
        val input = SecurityInputBuilder.build(listOf(a, b))
        assertEquals(listOf(1L, 2L), input.devices.map { it.id })
    }

    // ---- Hallazgos reconocidos (v1.3, ver el contrato § "Hallazgos reconocidos") ----

    @Test
    fun `sin acknowledged, SecurityInput trae la lista vacia`() {
        val input = SecurityInputBuilder.build(devices = emptyList())
        assertTrue(input.acknowledged.isEmpty())
    }

    @Test
    fun `acknowledged pasa tal cual al SecurityInput`() {
        val items = listOf(AcknowledgedFinding(findingId = "RISKY_SERVICE|192.168.0.55|554", at = now.toString()))
        val input = SecurityInputBuilder.build(devices = emptyList(), acknowledged = items)
        assertEquals(items, input.acknowledged)
    }
}
