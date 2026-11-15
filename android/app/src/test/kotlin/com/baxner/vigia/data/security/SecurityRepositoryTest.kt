package com.baxner.vigia.data.security

import com.baxner.vigia.data.agent.AgentApiClient
import com.baxner.vigia.data.agent.AgentEventStream
import com.baxner.vigia.data.agent.AgentHistoryRepository
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.network.CurrentNetwork
import java.time.Instant
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * El límite de refrescos de [SecurityRepository.refresh] (ver la revisión
 * de la fase 5, puntos 1 y 9): sin `force`, nunca antes de
 * [SecurityRepository.MinRefreshInterval] del último éxito; con `force`,
 * siempre. Usa el constructor primario (sin [android.content.Context], ver
 * la nota grande en la clase) con [activeAgentRepository] en `null` — modo
 * local, sin agente —, así no hace falta Robolectric ni un
 * [AgentHistoryRepository] real.
 */
class SecurityRepositoryTest {
    private val network = CurrentNetwork(
        cidr = "192.168.0.23/24", gateway = "192.168.0.1", isWifi = true, ssid = "Casa",
        bssid = null, rssi = null, frequencyMhz = null, linkSpeedMbps = null,
    )

    /** Cada [report] devuelve un puntaje distinto, para notar si de verdad se volvió a calcular (y no solo se devolvió el caché). */
    private class CountingEngine : SecurityEngine {
        var reportCalls = 0
        override val isAvailable = true
        override fun report(input: SecurityInput): SecurityReport? {
            reportCalls++
            return SecurityReport(score = 100 - reportCalls)
        }
        override fun routerInfo(gateway: String): RouterInfo? = null
        override fun portCatalog(): List<PortCatalogEntry> = emptyList()
    }

    private class InMemorySecurityStore : SecurityStore {
        private var last: RouterInfo? = null
        private var acknowledged: Map<String, List<AcknowledgedFinding>> = emptyMap()
        override fun lastRouterInfo(): RouterInfo? = last
        override fun setLastRouterInfo(info: RouterInfo) {
            last = info
        }

        override fun acknowledgedFindings(network: com.baxner.vigia.data.history.NetworkKey): List<AcknowledgedFinding> =
            acknowledged[network.value].orEmpty()

        override fun setAcknowledgedFindings(network: com.baxner.vigia.data.history.NetworkKey, items: List<AcknowledgedFinding>) {
            acknowledged = acknowledged + (network.value to items)
        }
    }

    private fun repositoryWithClock(engine: CountingEngine, clock: () -> Instant) = SecurityRepository(
        activeAgentRepository = MutableStateFlow(null),
        engine = engine,
        store = InMemorySecurityStore(),
        clock = clock,
    )

    @Test
    fun `sin informe previo, la primera llamada siempre calcula aunque no sea force`() = runTest {
        val engine = CountingEngine()
        val repo = repositoryWithClock(engine) { Instant.parse("2026-10-08T12:00:00Z") }

        repo.refresh(emptyList(), network, null)

        assertEquals(1, engine.reportCalls)
    }

    @Test
    fun `dentro de 60 s sin force, no vuelve a calcular y devuelve el mismo informe cacheado`() = runTest {
        var now = Instant.parse("2026-10-08T12:00:00Z")
        val engine = CountingEngine()
        val repo = repositoryWithClock(engine) { now }

        val first = repo.refresh(emptyList(), network, null).getOrThrow()
        now = now.plusSeconds(30)
        val second = repo.refresh(emptyList(), network, null).getOrThrow()

        assertEquals(1, engine.reportCalls)
        assertSame(first, second)
    }

    @Test
    fun `force vuelve a calcular aunque hayan pasado menos de 60 s`() = runTest {
        var now = Instant.parse("2026-10-08T12:00:00Z")
        val engine = CountingEngine()
        val repo = repositoryWithClock(engine) { now }

        repo.refresh(emptyList(), network, null)
        now = now.plusSeconds(5)
        repo.refresh(emptyList(), network, null, force = true)

        assertEquals(2, engine.reportCalls)
    }

    @Test
    fun `pasados 60 s, un refresco sin force vuelve a calcular`() = runTest {
        var now = Instant.parse("2026-10-08T12:00:00Z")
        val engine = CountingEngine()
        val repo = repositoryWithClock(engine) { now }

        repo.refresh(emptyList(), network, null)
        now = now.plusSeconds(61)
        repo.refresh(emptyList(), network, null)

        assertEquals(2, engine.reportCalls)
    }

    @Test
    fun `un segundo antes de los 60 s todavia no vuelve a calcular`() = runTest {
        var now = Instant.parse("2026-10-08T12:00:00Z")
        val engine = CountingEngine()
        val repo = repositoryWithClock(engine) { now }

        repo.refresh(emptyList(), network, null)
        now = now.plusSeconds(59)
        repo.refresh(emptyList(), network, null)

        assertEquals(1, engine.reportCalls)
    }

    @Test
    fun `a los 60 s exactos ya no cuenta como menos de 60 s y vuelve a calcular`() = runTest {
        var now = Instant.parse("2026-10-08T12:00:00Z")
        val engine = CountingEngine()
        val repo = repositoryWithClock(engine) { now }

        repo.refresh(emptyList(), network, null)
        now = now.plusSeconds(60)
        repo.refresh(emptyList(), network, null)

        assertEquals(2, engine.reportCalls)
    }

    @Test
    fun `el informe exitoso queda en el cache compartido`() = runTest {
        val engine = CountingEngine()
        val repo = repositoryWithClock(engine) { Instant.parse("2026-10-08T12:00:00Z") }

        assertNull(repo.report.value)
        val result = repo.refresh(emptyList(), network, null).getOrThrow()

        assertEquals(result, repo.report.value)
    }

    @Test
    fun `varios refrescos a la vez (Mutex) terminan calculando una sola vez dentro de los 60 s`() = runTest {
        val engine = CountingEngine()
        val repo = repositoryWithClock(engine) { Instant.parse("2026-10-08T12:00:00Z") }

        // 5 llamadas "a la vez" (mismo dispatcher de prueba, pero el Mutex
        // serializa igual): con el reloj fijo, solo la primera debería calcular.
        val jobs = List(5) { async { repo.refresh(emptyList(), network, null) } }
        val results = jobs.awaitAll()

        assertTrue(results.all { it.isSuccess })
        assertEquals(1, engine.reportCalls)
    }

    // ---- acknowledge/unacknowledge sin agente (ver el contrato § "Hallazgos reconocidos v1.3") ----

    @Test
    fun `acknowledge sin agente guarda el reconocimiento por red y refresh lo pasa al motor`() = runTest {
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
        val repo = SecurityRepository(MutableStateFlow(null), engine, InMemorySecurityStore()) { Instant.parse("2026-10-09T10:00:00Z") }

        val result = repo.acknowledge("RISKY_SERVICE|192.168.0.55|554", network)
        repo.refresh(emptyList(), network, null, force = true)

        assertTrue(result.isSuccess)
        assertEquals(listOf("RISKY_SERVICE|192.168.0.55|554"), seenAcknowledged?.map { it.findingId })
    }

    @Test
    fun `unacknowledge quita el reconocimiento guardado`() = runTest {
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
        val repo = SecurityRepository(MutableStateFlow(null), engine, InMemorySecurityStore()) { Instant.parse("2026-10-09T10:00:00Z") }

        repo.acknowledge("WIFI_WPS", network)
        repo.unacknowledge("WIFI_WPS", network)
        repo.refresh(emptyList(), network, null, force = true)

        assertTrue(seenAcknowledged.orEmpty().isEmpty())
    }

    @Test
    fun `acknowledge sin red utilizable falla`() = runTest {
        val engine = CountingEngine()
        val repo = SecurityRepository(MutableStateFlow(null), engine, InMemorySecurityStore()) { Instant.parse("2026-10-09T10:00:00Z") }

        val result = repo.acknowledge("WIFI_WPS", null)

        assertTrue(result.isFailure)
    }

    @Test
    fun `viendo remoto por Tailscale no mezcla la Wi-Fi actual con el informe del agente`() = runTest {
        val server = MockWebServer()
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"generatedAt":"2026-10-09T10:00:00Z","score":90}"""))
        server.start()
        try {
            val baseUrl = server.url("/").toString().trimEnd('/')
            val httpClient = OkHttpClient()
            val api = AgentApiClient(httpClient, baseUrl) { "token" }
            val stream = AgentEventStream(httpClient, baseUrl) { null }
            val cancelledScope = CoroutineScope(Job().apply { cancel() })
            val agentRepo = AgentHistoryRepository(api, stream, cancelledScope)

            var seenWifi: WifiInfo? = WifiInfo(ssid = "sentinel")
            val engine = object : SecurityEngine {
                override val isAvailable = true
                override fun report(input: SecurityInput): SecurityReport? {
                    seenWifi = input.wifi
                    return SecurityReport(score = 90)
                }
                override fun routerInfo(gateway: String): RouterInfo? = null
                override fun portCatalog(): List<PortCatalogEntry> = emptyList()
            }
            val repo = SecurityRepository(
                activeAgentRepository = MutableStateFlow(agentRepo),
                engine = engine,
                store = InMemorySecurityStore(),
                clock = { Instant.parse("2026-10-09T10:00:00Z") },
                isViewingRemote = { true },
            )

            repo.refresh(emptyList(), network, WifiInfo(ssid = "WiFi-Ajeno", security = "open"), force = true)

            assertNull(seenWifi)
        } finally {
            server.shutdown()
        }
    }
}
