package com.baxner.vigia.data.agent

import com.baxner.vigia.data.repository.VigiaJson
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Las formas JSON de Seguridad del contrato (ver `docs/agente-api.md` §
 * "Seguridad v1.2"): el ejemplo de `SecurityReport` del contrato (con los
 * mismos valores de muestra que usa esa sección) debe decodificar entero, y
 * `AlertSettingsDto`/`AgentNetworkDto.role` tienen que llevar las claves
 * nuevas de v1.2 sin romper lo que ya mandaba un agente v1.1.
 */
class SecurityDtoTest {

    /** El JSON de ejemplo de `SecurityReport` tal como lo da el contrato (mismos valores de muestra), con los tipos anidados resueltos a JSON válido. */
    private val contractSecurityReportJson = """
        {
          "generatedAt": "2026-10-08T15:00:00Z",
          "score": 82,
          "findings": [
            {
              "id": "RISKY_SERVICE|192.168.0.55|445",
              "kind": "RISKY_SERVICE",
              "severity": "high",
              "points": 8,
              "title": "Una computadora comparte archivos con toda la red",
              "detail": "PC-Escritorio (192.168.0.55) tiene abierto SMB (445).",
              "action": "Si no compartes carpetas desde esa computadora, desactiva el uso compartido.",
              "deviceId": 12,
              "ip": "192.168.0.55",
              "port": 445
            }
          ],
          "router": {
            "ip": "192.168.0.4",
            "mac": "02:00:00:10:20:01",
            "upnp": true,
            "natpmp": true,
            "externalIp": "192.168.100.2",
            "reach": "outer_ring",
            "portMappings": [
              {
                "protocol": "TCP",
                "externalPort": 8080,
                "internalIp": "192.168.0.55",
                "internalPort": 80,
                "description": "lo que dijo el equipo al pedirla",
                "source": "upnp",
                "deviceId": 12,
                "firstSeen": "2026-10-01T00:00:00Z",
                "lastSeen": "2026-10-08T15:00:00Z"
              }
            ]
          },
          "outerRing": {
            "network": "192.168.100.1|192.168.100.0/24",
            "subnet": "192.168.100.0/24",
            "gateway": "192.168.100.1",
            "lastScanAt": "2026-10-08T14:50:00Z",
            "devices": []
          }
        }
    """.trimIndent()

    @Test
    fun `el SecurityReport de ejemplo del contrato decodifica entero`() {
        val report = VigiaJson.decodeFromString<SecurityReport>(contractSecurityReportJson)

        assertEquals(82, report.score)
        assertEquals(1, report.findings.size)

        val finding = report.findings.single()
        assertEquals("RISKY_SERVICE|192.168.0.55|445", finding.id)
        assertEquals("RISKY_SERVICE", finding.kind)
        assertEquals(FindingSeverity.HIGH, finding.severityOrInfo())
        assertEquals(8, finding.points)
        assertEquals(12L, finding.deviceId)
        assertEquals(445, finding.port)

        val router = requireNotNull(report.router)
        assertEquals("192.168.0.4", router.ip)
        assertTrue(router.upnp)
        assertTrue(router.natpmp)
        assertEquals("192.168.100.2", router.externalIp)
        assertEquals(RouterReach.OUTER_RING, router.reachOrUnknown())
        val mapping = router.portMappings.single()
        assertEquals(8080, mapping.externalPort)
        assertEquals("192.168.0.55", mapping.internalIp)
        assertEquals(80, mapping.internalPort)
        assertEquals(PortMappingSource.UPNP, mapping.sourceOrUnknown())

        val outerRing = requireNotNull(report.outerRing)
        assertEquals("192.168.100.0/24", outerRing.subnet)
        assertEquals("192.168.100.1", outerRing.gateway)
        assertTrue(outerRing.devices.isEmpty())
    }

    @Test
    fun `router null cuando el agente no tiene UPnP ni NAT-PMP`() {
        val report = VigiaJson.decodeFromString<SecurityReport>(
            """{"generatedAt":"2026-10-08T15:00:00Z","score":100,"findings":[],"router":null,"outerRing":null}""",
        )
        assertNull(report.router)
        assertNull(report.outerRing)
    }

    @Test
    fun `AgentNetworkDto sin role de un agente v1 1 cae en inner`() {
        val dto = VigiaJson.decodeFromString<AgentNetworkDto>(
            """{"key":"k","subnet":"192.168.0.0/24","gateway":"192.168.0.1"}""",
        )
        assertEquals("inner", dto.role)
    }

    @Test
    fun `AgentNetworkDto con role outer lo conserva`() {
        val dto = VigiaJson.decodeFromString<AgentNetworkDto>(
            """{"key":"k","subnet":"192.168.100.0/24","gateway":"192.168.100.1","role":"outer"}""",
        )
        assertEquals("outer", dto.role)
    }

    @Test
    fun `AlertSettingsDto por defecto trae las 9 claves encendidas`() {
        val defaults = AlertSettingsDto()
        assertTrue(defaults.routerChanged)
        assertTrue(defaults.arpConflict)
        assertTrue(defaults.portMappingAdded)
        assertTrue(defaults.riskyService)
        AlertType.entries.forEach { type -> assertTrue("$type debería venir encendido", defaults.isEnabled(type)) }
    }

    @Test
    fun `AlertSettingsDto decodifica las 4 claves nuevas de Seguridad`() {
        val json = """
            {"NEW_DEVICE":true,"IMPORTANT_OFFLINE":true,"IMPORTANT_ONLINE":true,
             "AGENT_BLIND":true,"AGENT_RECOVERED":true,
             "ROUTER_CHANGED":false,"ARP_CONFLICT":false,"PORT_MAPPING_ADDED":false,"RISKY_SERVICE":false}
        """.trimIndent()
        val settings = VigiaJson.decodeFromString<AlertSettingsDto>(json)

        assertFalse(settings.isEnabled(AlertType.ROUTER_CHANGED))
        assertFalse(settings.isEnabled(AlertType.ARP_CONFLICT))
        assertFalse(settings.isEnabled(AlertType.PORT_MAPPING_ADDED))
        assertFalse(settings.isEnabled(AlertType.RISKY_SERVICE))
    }

    @Test
    fun `AlertSettingsDto de un agente v1 1 (sin las 4 claves nuevas) las deja en true`() {
        val json = """{"NEW_DEVICE":false,"IMPORTANT_OFFLINE":true,"IMPORTANT_ONLINE":true,"AGENT_BLIND":true,"AGENT_RECOVERED":true}"""
        val settings = VigiaJson.decodeFromString<AlertSettingsDto>(json)

        assertFalse(settings.isEnabled(AlertType.NEW_DEVICE))
        assertTrue(settings.isEnabled(AlertType.ROUTER_CHANGED))
        assertTrue(settings.isEnabled(AlertType.RISKY_SERVICE))
    }

    @Test
    fun `with actualiza cada uno de los 4 tipos nuevos sin tocar los demas`() {
        val base = AlertSettingsDto()
        val updated = base.with(AlertType.RISKY_SERVICE, enabled = false)

        assertFalse(updated.isEnabled(AlertType.RISKY_SERVICE))
        assertTrue(updated.isEnabled(AlertType.ROUTER_CHANGED))
        assertTrue(updated.isEnabled(AlertType.NEW_DEVICE))
    }

    @Test
    fun `AlertType reconoce los 4 tipos nuevos de Seguridad por texto`() {
        assertEquals(AlertType.ROUTER_CHANGED, AlertType.fromRaw("ROUTER_CHANGED"))
        assertEquals(AlertType.ARP_CONFLICT, AlertType.fromRaw("ARP_CONFLICT"))
        assertEquals(AlertType.PORT_MAPPING_ADDED, AlertType.fromRaw("PORT_MAPPING_ADDED"))
        assertEquals(AlertType.RISKY_SERVICE, AlertType.fromRaw("RISKY_SERVICE"))
    }

    @Test
    fun `SecurityReport sobrevive a una campo desconocido de una version futura`() {
        val report = VigiaJson.decodeFromString<SecurityReport>(
            """{"generatedAt":"x","score":90,"findings":[],"router":null,"outerRing":null,"futureField":123}""",
        )
        assertEquals(90, report.score)
    }

    // ---- Hallazgos reconocidos (v1.3, ver el contrato § "Hallazgos reconocidos") ----

    @Test
    fun `Finding sin acknowledged (agente anterior a v1 3) cae en false y null`() {
        val finding = VigiaJson.decodeFromString<Finding>(
            """{"id":"RISKY_SERVICE|192.168.0.55|445","kind":"RISKY_SERVICE","severity":"high","points":8,"title":"t","detail":"d"}""",
        )
        assertFalse(finding.acknowledged)
        assertNull(finding.acknowledgedAt)
    }

    @Test
    fun `Finding reconocido decodifica acknowledged y acknowledgedAt`() {
        val finding = VigiaJson.decodeFromString<Finding>(
            """{"id":"RISKY_SERVICE|dev:7|445","kind":"RISKY_SERVICE","severity":"high","points":8,"title":"t","detail":"d",
               "acknowledged":true,"acknowledgedAt":"2026-10-09T10:00:00Z"}""",
        )
        assertTrue(finding.acknowledged)
        assertEquals("2026-10-09T10:00:00Z", finding.acknowledgedAt)
    }

    @Test
    fun `AlertSettingsDto hace round trip completo`() {
        val settings = AlertSettingsDto(riskyService = false, routerChanged = false)
        val json = VigiaJson.encodeToString(settings)
        val decoded = VigiaJson.decodeFromString<AlertSettingsDto>(json)
        assertEquals(settings, decoded)
    }
}
