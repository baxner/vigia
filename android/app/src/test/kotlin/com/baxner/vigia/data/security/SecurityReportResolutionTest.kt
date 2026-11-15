package com.baxner.vigia.data.security

import com.baxner.vigia.data.agent.AgentApiException
import com.baxner.vigia.data.agent.Finding
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.network.CurrentNetwork
import java.io.IOException
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * [resolveMonitoredSecurityReport]/[resolveLocalSecurityReport] (ver la
 * revisión de la fase 5, punto 9: "pruebas para... el plan B con 404") y
 * [isSecurityEndpointMissing]: todo esto es libre de Android/Context a
 * propósito (ver la nota en [SecurityRepository]), así que se prueba con
 * JUnit plano, inyectando un [SecurityEngine] de prueba y el `security()`
 * del agente como un lambda — sin un [com.baxner.vigia.data.agent.AgentHistoryRepository] real.
 */
class SecurityReportResolutionTest {
    private val network = CurrentNetwork(
        cidr = "192.168.0.23/24", gateway = "192.168.0.1", isWifi = true, ssid = "Casa",
        bssid = null, rssi = null, frequencyMhz = null, linkSpeedMbps = null,
    )

    private class CountingEngine(
        override val isAvailable: Boolean = true,
        private val reportResult: SecurityReport? = SecurityReport(score = 77),
        private val routerInfoResult: RouterInfo? = null,
    ) : SecurityEngine {
        var reportCalls = 0
        var routerInfoCalls = 0

        override fun report(input: SecurityInput): SecurityReport? {
            reportCalls++
            return reportResult
        }

        override fun routerInfo(gateway: String): RouterInfo? {
            routerInfoCalls++
            return routerInfoResult
        }

        override fun portCatalog(): List<PortCatalogEntry> = emptyList()
    }

    // ---- isSecurityEndpointMissing ----

    @Test
    fun `404 es endpoint faltante`() {
        assertTrue(AgentApiException(404, "no existe").isSecurityEndpointMissing())
    }

    @Test
    fun `501 tambien es endpoint faltante`() {
        assertTrue(AgentApiException(501, "no implementado").isSecurityEndpointMissing())
    }

    @Test
    fun `401 o 500 del agente no son endpoint faltante`() {
        assertFalse(AgentApiException(401, "sin token").isSecurityEndpointMissing())
        assertFalse(AgentApiException(500, "error interno").isSecurityEndpointMissing())
    }

    @Test
    fun `un error que no es del agente (p ej sin red) nunca es endpoint faltante`() {
        assertFalse(IOException("tiempo de espera agotado").isSecurityEndpointMissing())
        assertFalse((null as Throwable?).isSecurityEndpointMissing())
    }

    // ---- resolveMonitoredSecurityReport: el agente contesta (plan A) ----

    @Test
    fun `agente contesta y motor no disponible, muestra el informe del agente tal cual`() = runTest {
        val agentReport = SecurityReport(score = 90)
        val engine = CountingEngine(isAvailable = false)

        val result = resolveMonitoredSecurityReport({ Result.success(agentReport) }, engine, emptyList(), network, null, emptyList())

        assertEquals(Result.success(agentReport), result)
        assertEquals(0, engine.reportCalls)
    }

    @Test
    fun `agente contesta y motor disponible, arma el informe completo con la Wi-Fi`() {
        runTest {
            val agentReport = SecurityReport(score = 90, router = RouterInfo(ip = "192.168.0.4"))
            val merged = SecurityReport(score = 70)
            val engine = CountingEngine(reportResult = merged)

            val result = resolveMonitoredSecurityReport(
                { Result.success(agentReport) }, engine, emptyList(), network, WifiInfo(security = "wep"), emptyList(),
            )

            assertEquals(Result.success(merged), result)
            assertEquals(1, engine.reportCalls)
            // En Red monitoreada el router ya lo dio el agente: no hace falta sondearlo otra vez.
            assertEquals(0, engine.routerInfoCalls)
        }
    }

    @Test
    fun `si el motor no logra calcular nada, se queda con el informe del agente`() = runTest {
        val agentReport = SecurityReport(score = 90)
        val engine = CountingEngine(reportResult = null)

        val result = resolveMonitoredSecurityReport({ Result.success(agentReport) }, engine, emptyList(), network, null, emptyList())

        assertEquals(Result.success(agentReport), result)
    }

    // ---- resolveMonitoredSecurityReport: 404/501 del agente (plan B) ----

    @Test
    fun `404 del agente con motor disponible calcula el informe local`() = runTest {
        val local = SecurityReport(score = 65)
        val engine = CountingEngine(reportResult = local, routerInfoResult = RouterInfo(ip = "192.168.100.1"))

        val result = resolveMonitoredSecurityReport(
            { Result.failure(AgentApiException(404, "no existe")) }, engine, emptyList(), network, null, emptyList(),
        )

        assertEquals(Result.success(local), result)
        assertEquals(1, engine.routerInfoCalls)
        assertEquals(1, engine.reportCalls)
    }

    @Test
    fun `501 del agente tambien dispara el plan B`() = runTest {
        val local = SecurityReport(score = 50)
        val engine = CountingEngine(reportResult = local)

        val result = resolveMonitoredSecurityReport(
            { Result.failure(AgentApiException(501, "no implementado")) }, engine, emptyList(), network, null, emptyList(),
        )

        assertTrue(result.isSuccess)
        assertEquals(local, result.getOrNull())
    }

    @Test
    fun `404 sin motor disponible falla (no hay plan B posible)`() = runTest {
        val engine = CountingEngine(isAvailable = false)
        val error = AgentApiException(404, "no existe")

        val result = resolveMonitoredSecurityReport({ Result.failure(error) }, engine, emptyList(), network, null, emptyList())

        assertEquals(error, result.exceptionOrNull())
    }

    @Test
    fun `un error de red de verdad no dispara el plan B y se propaga (agentUnreachable)`() = runTest {
        val engine = CountingEngine()
        val error = IOException("tiempo de espera agotado")

        val result = resolveMonitoredSecurityReport({ Result.failure(error) }, engine, emptyList(), network, null, emptyList())

        assertEquals(error, result.exceptionOrNull())
        assertEquals(0, engine.reportCalls)
    }

    @Test
    fun `un 401 del agente tampoco dispara el plan B`() = runTest {
        val engine = CountingEngine()
        val error = AgentApiException(401, "sin token")

        val result = resolveMonitoredSecurityReport({ Result.failure(error) }, engine, emptyList(), network, null, emptyList())

        assertEquals(error, result.exceptionOrNull())
        assertEquals(0, engine.reportCalls)
    }

    // ---- resolveLocalSecurityReport ----

    @Test
    fun `sin agente y sin motor disponible, falla con SecurityUnavailableException`() = runTest {
        val engine = CountingEngine(isAvailable = false)

        val result = resolveLocalSecurityReport(engine, emptyList(), network, null, emptyList())

        assertTrue(result.exceptionOrNull() is SecurityUnavailableException)
    }

    @Test
    fun `sin agente, con motor disponible, sondea el router del gateway actual`() = runTest {
        val local = SecurityReport(score = 80)
        val engine = CountingEngine(reportResult = local, routerInfoResult = RouterInfo(ip = "192.168.0.1"))

        val result = resolveLocalSecurityReport(engine, emptyList(), network, null, emptyList())

        assertEquals(Result.success(local), result)
        assertEquals(1, engine.routerInfoCalls)
    }

    @Test
    fun `sin agente y sin gateway, no intenta sondear el router`() = runTest {
        val local = SecurityReport(score = 80)
        val engine = CountingEngine(reportResult = local)
        val sinGateway = network.copy(gateway = null)

        resolveLocalSecurityReport(engine, emptyList(), sinGateway, null, emptyList())

        assertEquals(0, engine.routerInfoCalls)
    }

    @Test
    fun `si el motor local no logra calcular nada, falla con SecurityUnavailableException`() = runTest {
        val engine = CountingEngine(reportResult = null)

        val result = resolveLocalSecurityReport(engine, emptyList(), network, null, emptyList())

        assertTrue(result.exceptionOrNull() is SecurityUnavailableException)
    }

    @Test
    fun `resolveLocalSecurityReport pasa acknowledged al SecurityInput`() = runTest {
        var seenAcknowledged: List<AcknowledgedFinding>? = null
        val engine = object : SecurityEngine {
            override val isAvailable = true
            override fun report(input: SecurityInput): SecurityReport? {
                seenAcknowledged = input.acknowledged
                return SecurityReport(score = 90)
            }
            override fun routerInfo(gateway: String): RouterInfo? = null
            override fun portCatalog(): List<PortCatalogEntry> = emptyList()
        }
        val ack = listOf(AcknowledgedFinding(findingId = "WIFI_WPS", at = "2026-10-09T10:00:00Z"))

        resolveLocalSecurityReport(engine, emptyList(), network, null, emptyList(), ack)

        assertEquals(ack, seenAcknowledged)
    }

    @Test
    fun `resolveMonitoredSecurityReport conserva los hallazgos reconocidos del agente cuando el motor esta disponible`() = runTest {
        var seenAcknowledged: List<AcknowledgedFinding>? = null
        val agentReport = SecurityReport(
            score = 90,
            findings = listOf(
                Finding(
                    id = "RISKY_SERVICE|192.168.0.55|445",
                    kind = "RISKY_SERVICE",
                    severity = "high",
                    points = 10,
                    title = "SMB abierto",
                    detail = "SMB (445)",
                    action = "Cerrar",
                    acknowledged = true,
                    acknowledgedAt = "2026-10-09T10:00:00Z",
                ),
                Finding(
                    id = "WIFI_WPS",
                    kind = "WIFI_WPS",
                    severity = "medium",
                    points = 5,
                    title = "WPS activo",
                    detail = "WPS",
                    action = "Apagar",
                    acknowledged = false,
                ),
            ),
        )
        val engine = object : SecurityEngine {
            override val isAvailable = true
            override fun report(input: SecurityInput): SecurityReport? {
                seenAcknowledged = input.acknowledged
                return SecurityReport(score = 85, findings = agentReport.findings)
            }
            override fun routerInfo(gateway: String): RouterInfo? = null
            override fun portCatalog(): List<PortCatalogEntry> = emptyList()
        }

        val result = resolveMonitoredSecurityReport({ Result.success(agentReport) }, engine, emptyList(), network, null, emptyList())

        assertTrue(result.isSuccess)
        assertEquals(
            listOf(AcknowledgedFinding(findingId = "RISKY_SERVICE|192.168.0.55|445", at = "2026-10-09T10:00:00Z")),
            seenAcknowledged,
        )
    }

    // ---- localNetworkKeyFor (ver el contrato § "Hallazgos reconocidos v1.3": "la app sin agente los guarda... por red") ----

    @Test
    fun `localNetworkKeyFor combina ssid, gateway y subred`() {
        val key = localNetworkKeyFor(network)
        assertEquals("Casa|192.168.0.1|192.168.0.0/24", key?.value)
    }

    @Test
    fun `localNetworkKeyFor es null sin subred utilizable`() {
        assertEquals(null, localNetworkKeyFor(network.copy(cidr = null)))
        assertEquals(null, localNetworkKeyFor(null))
    }

    @Test
    fun `localNetworkKeyFor da claves distintas para gateways distintos`() {
        val a = localNetworkKeyFor(network)
        val b = localNetworkKeyFor(network.copy(gateway = "192.168.0.254"))
        assertTrue(a != b) // el gateway forma parte de la clave: una red distinta no debe compartir reconocimientos.
    }
}
