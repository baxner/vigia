package com.baxner.vigia.data.agent

import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * TLS v1.5 (ver `docs/agente-api.md`, "TLS (v1.5)"): el `TrustManager` fijado
 * por huella ([AgentTls.pinnedSocketFactory]) contra un servidor HTTPS de
 * verdad (MockWebServer + un certificado de prueba generado en el test, ver
 * el encargo), y la migración de un vínculo de antes de esta fase
 * ([migrateLegacyLinkIfNeeded]).
 */
class AgentTlsTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun `pinnedSocketFactory acepta el certificado cuya huella coincide`() = runTest {
        val serverCert = HeldCertificate.Builder().build()
        val serverHandshake = HandshakeCertificates.Builder().heldCertificate(serverCert).build()
        server.useHttps(serverHandshake.sslSocketFactory(), false)
        server.enqueue(MockResponse().setResponseCode(200).setBody("ok"))
        server.start()

        val fingerprint = AgentTls.fingerprintOf(serverCert.certificate)
        val client = pinnedClient(fingerprint)

        val response = client.newCall(Request.Builder().url(server.url("/")).build()).execute()
        assertEquals(200, response.code)
        response.close()
    }

    @Test
    fun `pinnedSocketFactory rechaza un certificado con otra huella`() = runTest {
        val serverCert = HeldCertificate.Builder().build()
        val otroCert = HeldCertificate.Builder().build() // un certificado de prueba DISTINTO: el que de verdad presenta el servidor es serverCert.
        val serverHandshake = HandshakeCertificates.Builder().heldCertificate(serverCert).build()
        server.useHttps(serverHandshake.sslSocketFactory(), false)
        server.enqueue(MockResponse().setResponseCode(200).setBody("ok"))
        server.start()

        // Se fija a la huella de OTRO certificado: el que presenta el servidor no coincide.
        val fingerprint = AgentTls.fingerprintOf(otroCert.certificate)
        val client = pinnedClient(fingerprint)

        val thrown = runCatching {
            client.newCall(Request.Builder().url(server.url("/")).build()).execute().close()
        }.exceptionOrNull()

        assertNotNull("esperaba que la conexión fallara (huella distinta)", thrown)
        assertTrue("esperaba un CertPinMismatchException en la cadena de causas", thrown!!.isCertPinMismatch())
    }

    @Test
    fun `isCertPinMismatch busca en toda la cadena de causas`() {
        val mismatch = CertPinMismatchException("huella distinta")
        val wrapped = RuntimeException("falló el handshake", mismatch)
        val doblementeEnvuelto = IllegalStateException("falló la llamada", wrapped)

        assertTrue(doblementeEnvuelto.isCertPinMismatch())
        assertFalse(RuntimeException("otro error cualquiera").isCertPinMismatch())
    }

    @Test
    fun `fingerprintOf da 64 caracteres hex en minusculas`() {
        val cert = HeldCertificate.Builder().build()
        val fingerprint = AgentTls.fingerprintOf(cert.certificate)

        assertEquals(64, fingerprint.length)
        assertEquals(fingerprint, fingerprint.lowercase())
        assertTrue(fingerprint.all { it in "0123456789abcdef" })
    }

    // ---- migrateLegacyLinkIfNeeded (ver AgentTls.kt) ----

    @Test
    fun `vinculo sin huella la toma de info por HTTP y pasa a tener certSha256`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"agent-1","name":"Mac-Estudio","certSha256":"ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34"}""",
            ),
        )
        server.start()
        val serverUrl = server.url("/")
        val linkStore = FakeAgentLinkStore()
        val sinHuella = AgentLink(agentId = "agent-1", agentName = "Mac-Estudio", host = serverUrl.host, port = serverUrl.port, token = "t")
        linkStore.save(sinHuella)

        // La vía vieja es HTTP en el mismo puerto del vínculo (ver
        // AgentTls.legacyHttpBaseUrl): el MockWebServer habla HTTP plano.
        val migrated = migrateLegacyLinkIfNeeded(linkStore, sinHuella, OkHttpClient())

        assertNotNull(migrated)
        assertEquals("ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34", migrated!!.certSha256)
        assertEquals(migrated.certSha256, linkStore.get()?.certSha256)
    }

    @Test
    fun `vinculo con huella no hace ninguna llamada de red`() = runTest {
        server.start()
        server.shutdown() // nada escuchando: si migrateLegacyLinkIfNeeded intentara conectar, fallaría y devolvería null.
        val linkStore = FakeAgentLinkStore()
        val conHuella = AgentLink(
            agentId = "agent-1", agentName = "Mac-Estudio", host = "127.0.0.1", port = 7341, token = "t",
            certSha256 = "ya-fijada",
        )
        linkStore.save(conHuella)

        val result = migrateLegacyLinkIfNeeded(linkStore, conHuella, OkHttpClient())

        assertEquals(conHuella, result)
    }

    @Test
    fun `vinculo sin huella y agente inalcanzable devuelve null, no un error`() = runTest {
        server.start()
        val serverUrl = server.url("/")
        server.shutdown() // ahora nada escucha ahí: simula "todavía no migrado, agente apagado".
        val linkStore = FakeAgentLinkStore()
        val sinHuella = AgentLink(agentId = "agent-1", agentName = "Mac-Estudio", host = serverUrl.host, port = serverUrl.port, token = "t")
        linkStore.save(sinHuella)

        val result = migrateLegacyLinkIfNeeded(linkStore, sinHuella, OkHttpClient())

        assertNull(result)
        assertNull(linkStore.get()?.certSha256)
    }

    // ---- resolvePinnedReachableAgent / withPinnedReachableAgent (ver el encargo, fase 8: "reutiliza withPinnedReachableAgent/AgentReachability en vez de duplicar la lógica") ----

    @Test
    fun `resolvePinnedReachableAgent devuelve el cliente que contesto y el resultado de probe`() = runTest {
        val serverCert = HeldCertificate.Builder().build()
        val serverHandshake = HandshakeCertificates.Builder().heldCertificate(serverCert).build()
        server.useHttps(serverHandshake.sslSocketFactory(), false)
        server.enqueue(MockResponse().setResponseCode(200).setBody("""[{"key":"k","subnet":"192.168.0.0/24","gateway":"192.168.0.1"}]"""))
        server.start()
        val serverUrl = server.url("/")
        val fingerprint = AgentTls.fingerprintOf(serverCert.certificate)
        val linkStore = FakeAgentLinkStore()
        linkStore.save(
            AgentLink(agentId = "agent-1", agentName = "Mac-Estudio", host = serverUrl.host, port = serverUrl.port, token = "t", certSha256 = fingerprint),
        )

        val result = resolvePinnedReachableAgent(linkStore, emptyList(), OkHttpClient()) { client -> client.networks() }

        assertTrue(result.isSuccess)
        val (client, networks) = result.getOrThrow()
        assertEquals("https://${serverUrl.host}:${serverUrl.port}", client.baseUrl)
        assertEquals(1, networks.size)
        assertEquals("k", networks.first().key)
    }

    @Test
    fun `withPinnedReachableAgent es un envoltorio fino de resolvePinnedReachableAgent`() = runTest {
        val serverCert = HeldCertificate.Builder().build()
        val serverHandshake = HandshakeCertificates.Builder().heldCertificate(serverCert).build()
        server.useHttps(serverHandshake.sslSocketFactory(), false)
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"agent-1","name":"Mac-Estudio"}"""))
        server.start()
        val serverUrl = server.url("/")
        val fingerprint = AgentTls.fingerprintOf(serverCert.certificate)
        val linkStore = FakeAgentLinkStore()
        linkStore.save(
            AgentLink(agentId = "agent-1", agentName = "Mac-Estudio", host = serverUrl.host, port = serverUrl.port, token = "t", certSha256 = fingerprint),
        )

        val result = withPinnedReachableAgent(linkStore, emptyList(), OkHttpClient()) { it.info() }

        assertTrue(result.isSuccess)
        assertEquals("Mac-Estudio", result.getOrThrow().name)
    }

    @Test
    fun `resolvePinnedReachableAgent propaga el fallo de probe (p ej 404 501) sin reintentar como si fuera de red`() = runTest {
        val serverCert = HeldCertificate.Builder().build()
        val serverHandshake = HandshakeCertificates.Builder().heldCertificate(serverCert).build()
        server.useHttps(serverHandshake.sslSocketFactory(), false)
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"agent-1","name":"Mac-Estudio"}"""))
        server.start()
        val serverUrl = server.url("/")
        val fingerprint = AgentTls.fingerprintOf(serverCert.certificate)
        val linkStore = FakeAgentLinkStore()
        linkStore.save(
            AgentLink(agentId = "agent-1", agentName = "Mac-Estudio", host = serverUrl.host, port = serverUrl.port, token = "t", certSha256 = fingerprint),
        )

        val result = resolvePinnedReachableAgent(linkStore, emptyList(), OkHttpClient()) {
            Result.failure<AgentInfoDto>(AgentApiException(501, "todavía no"))
        }

        assertTrue(result.isFailure)
        assertEquals(501, (result.exceptionOrNull() as? AgentApiException)?.status)
    }

    @Test
    fun `huella distinta en la IP de la LAN cuenta como no contesta, no como certificado cambiado`() = runTest {
        val serverCert = HeldCertificate.Builder().build()
        val otroCert = HeldCertificate.Builder().build()
        val serverHandshake = HandshakeCertificates.Builder().heldCertificate(serverCert).build()
        server.useHttps(serverHandshake.sslSocketFactory(), false)
        server.enqueue(MockResponse().setResponseCode(200).setBody("[]"))
        server.start()
        val serverUrl = server.url("/")
        // Fijado a la huella de OTRO certificado: el que presenta el servidor no coincide.
        val fingerprint = AgentTls.fingerprintOf(otroCert.certificate)
        val linkStore = FakeAgentLinkStore()
        linkStore.save(
            AgentLink(agentId = "agent-1", agentName = "Mac-Estudio", host = serverUrl.host, port = serverUrl.port, token = "t", certSha256 = fingerprint),
        )

        val result = resolvePinnedReachableAgent(linkStore, emptyList(), OkHttpClient()) { it.networks() }

        // 127.0.0.1 no es Tailscale: en otra red con la misma numeración, en
        // esa IP puede contestar cualquier equipo con su propio certificado.
        assertTrue(result.isFailure)
        assertFalse(result.exceptionOrNull()!!.isCertPinMismatch())
        assertEquals(404, (result.exceptionOrNull() as? AgentApiException)?.status)
    }

    @Test
    fun `huella distinta por Tailscale si es certificado cambiado`() {
        val mismatch = javax.net.ssl.SSLHandshakeException("handshake").apply { initCause(CertPinMismatchException("otra huella")) }
        assertTrue(mismatch.unlessForeignLanCert("https://100.64.0.10:7341").isCertPinMismatch())
        assertFalse(mismatch.unlessForeignLanCert("https://192.168.0.48:7341").isCertPinMismatch())
        val otro = RuntimeException("timeout")
        assertEquals(otro, otro.unlessForeignLanCert("https://192.168.0.48:7341"))
    }

    @Test
    fun `resolvePinnedReachableAgent sin vinculo falla sin tocar la red`() = runTest {
        val linkStore = FakeAgentLinkStore()

        val result = resolvePinnedReachableAgent(linkStore, emptyList(), OkHttpClient()) { it.networks() }

        assertTrue(result.isFailure)
    }

    private fun pinnedClient(fingerprint: String): OkHttpClient {
        val (sslSocketFactory, trustManager) = AgentTls.pinnedSocketFactory(fingerprint)
        return OkHttpClient.Builder()
            .sslSocketFactory(sslSocketFactory, trustManager)
            .hostnameVerifier { _, _ -> true } // ver TLS v1.5: nunca se valida el hostname.
            .build()
    }
}

/** [AgentLinkStore] en memoria, para probar la migración sin EncryptedSharedPreferences (ver AgentLink.kt). */
private class FakeAgentLinkStore : AgentLinkStore {
    private var current: AgentLink? = null
    override fun get(): AgentLink? = current
    override fun save(link: AgentLink) { current = link }
    override fun clear() { current = null }
}
