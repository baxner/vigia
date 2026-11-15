package com.baxner.vigia.data.repository

import com.baxner.vigia.data.model.ScanResult
import kotlinx.serialization.decodeFromString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * El JSON de ejemplo tiene exactamente la forma que describe mobile.go: la
 * mayoría de los campos son `omitempty` en Go, así que cualquier dispositivo
 * "pelado" (sin MAC, sin servicios, etc.) es un caso real y no una rareza del
 * test. También incluye una clave desconocida (`"engineBuild"`) y un
 * `"sources": null` explícito, para probar `ignoreUnknownKeys` y
 * `coerceInputValues` a la vez.
 */
class ScanResultParsingTest {

    private val sampleJson = """
        {
          "subnet": "192.168.0.0/24",
          "self": "192.168.0.105",
          "gateway": "192.168.0.4",
          "startedAt": "2026-10-05T14:32:08-03:00",
          "duration": 3400000000,
          "engineBuild": "dev-unknown-field",
          "devices": [
            {
              "ip": "192.168.0.4",
              "mac": "aa:bb:cc:dd:ee:ff",
              "vendor": "TP-Link",
              "hostname": "router.local",
              "identity": {
                "type": "router",
                "brand": "TP-Link",
                "confidence": 90,
                "reason": "mdns hostname=router.local; mac vendor=TP-Link"
              },
              "services": [
                {"proto": "mdns", "name": "_http._tcp", "info": "router.local"}
              ],
              "openPorts": [80, 443],
              "sources": ["arp", "mdns"],
              "extra": {"ssdp:friendlyName": "Router de casa"},
              "lastSeen": "2026-10-05T14:32:08-03:00"
            },
            {
              "ip": "192.168.0.50",
              "identity": {"type": "unknown", "confidence": 0},
              "sources": null
            }
          ]
        }
    """.trimIndent()

    @Test
    fun `decodifica el resultado completo`() {
        val result = VigiaJson.decodeFromString<ScanResult>(sampleJson)

        assertEquals("192.168.0.0/24", result.subnet)
        assertEquals("192.168.0.105", result.self)
        assertEquals("192.168.0.4", result.gateway)
        assertEquals(3_400_000_000L, result.duration)
        assertEquals(2, result.devices.size)
    }

    @Test
    fun `dispositivo completo trae todos sus campos`() {
        val result = VigiaJson.decodeFromString<ScanResult>(sampleJson)
        val router = result.devices.first { it.ip == "192.168.0.4" }

        assertEquals("aa:bb:cc:dd:ee:ff", router.mac)
        assertEquals("TP-Link", router.vendor)
        assertEquals("router.local", router.hostname)
        assertEquals("router", router.identity.type)
        assertEquals(90, router.identity.confidence)
        assertEquals(listOf(80, 443), router.openPorts)
        assertEquals(listOf("arp", "mdns"), router.sources)
        assertEquals(1, router.services.size)
        assertEquals("mdns", router.services.first().proto)
        assertEquals("Router de casa", router.extra["ssdp:friendlyName"])
    }

    @Test
    fun `dispositivo pelado usa los valores por defecto para los campos ausentes`() {
        val result = VigiaJson.decodeFromString<ScanResult>(sampleJson)
        val bare = result.devices.first { it.ip == "192.168.0.50" }

        assertEquals("", bare.mac)
        assertEquals("", bare.vendor)
        assertEquals("", bare.hostname)
        assertTrue(bare.ipv6.isEmpty())
        assertTrue(bare.services.isEmpty())
        assertTrue(bare.openPorts.isEmpty())
        assertTrue(bare.extra.isEmpty())
        assertEquals("", bare.lastSeen)
        assertEquals("unknown", bare.identity.type)
        assertEquals("", bare.identity.brand)
    }

    @Test
    fun `sources null (no ausente) cae en lista vacia gracias a coerceInputValues`() {
        val result = VigiaJson.decodeFromString<ScanResult>(sampleJson)
        val bare = result.devices.first { it.ip == "192.168.0.50" }

        assertTrue(bare.sources.isEmpty())
    }

    @Test
    fun `lista de puertos de scanPorts`() {
        val ports = VigiaJson.decodeFromString<List<Int>>("[22,80,443]")
        assertEquals(listOf(22, 80, 443), ports)
    }

    @Test
    fun `scan result vacio (sin red) no revienta`() {
        val result = VigiaJson.decodeFromString<ScanResult>("{}")
        assertEquals("", result.subnet)
        assertTrue(result.devices.isEmpty())
    }
}
