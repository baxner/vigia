package com.baxner.vigia.data.agent

import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * [AgentApiClient] contra MockWebServer: la vinculación, que el token SIEMPRE
 * vaya en la cabecera (nunca en la URL, ver el contrato), el PATCH con
 * ausente/`null`, y que los errores HTTP se conviertan en [AgentApiException]
 * con el mensaje que manda el agente.
 */
class AgentApiClientTest {
    private lateinit var server: MockWebServer
    private lateinit var client: AgentApiClient
    private var token: String? = "un-token-de-prueba"

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        client = AgentApiClient(OkHttpClient(), server.url("/").toString()) { token }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun `pair manda code y client en el cuerpo, sin cabecera de autorizacion`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"token":"abc123","agentId":"agent-1","agentName":"Mac-Estudio"}"""))

        val result = client.pair("482913", "Telefono-Android")

        val response = result.getOrThrow()
        assertEquals("abc123", response.token)
        assertEquals("Mac-Estudio", response.agentName)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/pair", request.path)
        assertNull(request.getHeader("Authorization"))
        assertTrue(request.body.readUtf8().let { it.contains("\"code\":\"482913\"") && it.contains("Telefono-Android") })
    }

    @Test
    fun `pair con codigo invalido devuelve el mensaje del agente`() = runTest {
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error":"código inválido o vencido"}"""))

        val result = client.pair("000000", "Telefono-Android")

        val error = result.exceptionOrNull() as? AgentApiException
        assertEquals(401, error?.status)
        assertEquals("código inválido o vencido", error?.message)
    }

    @Test
    fun `el token va en la cabecera Authorization y nunca en la URL`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("[]"))

        client.networks()

        val request = server.takeRequest()
        assertEquals("Bearer un-token-de-prueba", request.getHeader("Authorization"))
        assertFalse(request.path.orEmpty().contains("un-token-de-prueba"))
        assertFalse(request.requestUrl.toString().contains("un-token-de-prueba"))
    }

    @Test
    fun `sin token activo falla antes de llamar a la red`() = runTest {
        token = null

        val result = client.networks()

        assertEquals(0, server.requestCount)
        assertTrue(result.isFailure)
    }

    @Test
    fun `devicesForNetwork manda la clave de red url-encoded`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("[]"))

        client.devicesForNetwork("192.168.0.1|192.168.0.0/24")

        val request = server.takeRequest()
        assertEquals("/api/v1/networks/192.168.0.1%7C192.168.0.0%2F24/devices", request.path)
    }

    @Test
    fun `patchCustomization de un reemplazo completo manda los 5 campos`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":1,"network":"k","lastIp":"192.168.0.10","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-01T00:00:00Z","online":true,"last":{}}""",
            ),
        )

        val patch = DeviceCustomizationPatch.replaceAll(com.baxner.vigia.data.history.DeviceCustomization(name = "Foco", important = true))
        val result = client.patchCustomization(1, patch)

        assertEquals(1L, result.getOrThrow().id)
        val request = server.takeRequest()
        assertEquals("PATCH", request.method)
        assertEquals("Bearer un-token-de-prueba", request.getHeader("Authorization"))
        val body = request.body.readUtf8()
        assertTrue(body.contains("\"typeId\":null"))
        assertTrue(body.contains("\"name\":\"Foco\""))
    }

    @Test
    fun `patchCustomization de onlySet omite los campos ausentes del cuerpo`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":1,"network":"k","lastIp":"i","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-01T00:00:00Z","online":true,"last":{}}"""))

        val patch = DeviceCustomizationPatch.onlySet(com.baxner.vigia.data.history.DeviceCustomization(location = "Sala"))
        client.patchCustomization(1, patch)

        val body = server.takeRequest().body.readUtf8()
        assertEquals("""{"location":"Sala"}""", body)
    }

    @Test
    fun `forget hace DELETE y acepta 204 sin cuerpo`() = runTest {
        server.enqueue(MockResponse().setResponseCode(204))

        val result = client.forget(7)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/devices/7", request.path)
    }

    @Test
    fun `startScan 202 started true`() = runTest {
        server.enqueue(MockResponse().setResponseCode(202).setBody("""{"started":true}"""))

        assertTrue(client.startScan().getOrThrow())
    }

    @Test
    fun `startScan 409 cuando ya hay uno en curso`() = runTest {
        server.enqueue(MockResponse().setResponseCode(409).setBody("""{"error":"ya hay un escaneo en curso"}"""))

        val result = client.startScan()

        val error = result.exceptionOrNull() as? AgentApiException
        assertEquals(409, error?.status)
    }

    @Test
    fun `info no manda cabecera de autorizacion`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"agent-1","name":"Mac-Estudio"}"""))

        val info = client.info().getOrThrow()

        assertEquals("Mac-Estudio", info.name)
        assertNull(server.takeRequest().getHeader("Authorization"))
    }

    @Test
    fun `error sin cuerpo JSON reconocible cae a un mensaje generico`() = runTest {
        server.enqueue(MockResponse().setResponseCode(500).setBody("boom"))

        val error = client.networks().exceptionOrNull() as? AgentApiException

        assertEquals(500, error?.status)
        assertEquals("error HTTP 500", error?.message)
    }

    @Test
    fun `info decodifica addresses cuando el agente las manda`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"agent-1","name":"Mac-Estudio","addresses":["192.168.0.48","100.64.0.10"]}""",
            ),
        )

        val info = client.info().getOrThrow()

        assertEquals(listOf("192.168.0.48", "100.64.0.10"), info.addresses)
    }

    @Test
    fun `info sin addresses (agente v1_0) cae en lista vacia`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"agent-1","name":"Mac-Estudio"}"""))

        assertTrue(client.info().getOrThrow().addresses.isEmpty())
    }

    // ---- Alertas (v1.1, ver el contrato § "Alertas") ----

    @Test
    fun `alerts manda after y limit, token en la cabecera`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """[{"id":57,"type":"NEW_DEVICE","severity":"high","at":"2026-10-07T07:10:00Z","title":"Equipo nuevo en tu red","body":"Telefono-Invitado (192.168.0.24) se conectó por primera vez.","deviceId":31,"network":"k","read":false}]""",
            ),
        )

        val alerts = client.alerts(after = 10, limit = 50).getOrThrow()

        assertEquals(1, alerts.size)
        assertEquals(57L, alerts.first().id)
        assertEquals("NEW_DEVICE", alerts.first().type)
        assertEquals(31L, alerts.first().deviceId)
        val request = server.takeRequest()
        assertEquals("/api/v1/alerts?after=10&limit=50", request.path)
        assertEquals("Bearer un-token-de-prueba", request.getHeader("Authorization"))
    }

    @Test
    fun `alerts por defecto pide after 0 y limit 100`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("[]"))

        client.alerts()

        assertEquals("/api/v1/alerts?after=0&limit=100", server.takeRequest().path)
    }

    @Test
    fun `markRead manda upTo en el cuerpo y acepta 204`() = runTest {
        server.enqueue(MockResponse().setResponseCode(204))

        val result = client.markRead(upTo = 57)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/alerts/read", request.path)
        assertEquals("""{"upTo":57}""", request.body.readUtf8())
    }

    @Test
    fun `alertSettings decodifica los 5 interruptores`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"NEW_DEVICE":true,"IMPORTANT_OFFLINE":false,"IMPORTANT_ONLINE":true,"AGENT_BLIND":true,"AGENT_RECOVERED":false}""",
            ),
        )

        val settings = client.alertSettings().getOrThrow()

        assertTrue(settings.newDevice)
        assertFalse(settings.importantOffline)
        assertTrue(settings.importantOnline)
        assertTrue(settings.agentBlind)
        assertFalse(settings.agentRecovered)
    }

    @Test
    fun `putAlertSettings manda PUT con los 5 campos y el token`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"NEW_DEVICE":false,"IMPORTANT_OFFLINE":true,"IMPORTANT_ONLINE":true,"AGENT_BLIND":true,"AGENT_RECOVERED":true}""",
            ),
        )

        val result = client.putAlertSettings(AlertSettingsDto(newDevice = false))

        assertTrue(result.getOrThrow().newDevice == false)
        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/alert-settings", request.path)
        assertEquals("Bearer un-token-de-prueba", request.getHeader("Authorization"))
        assertTrue(request.body.readUtf8().contains("\"NEW_DEVICE\":false"))
    }

    // ---- Hallazgos reconocidos (v1.3, ver el contrato § "Hallazgos reconocidos") ----

    @Test
    fun `acknowledgeFinding manda PUT con el id url-encoded y el token`() = runTest {
        server.enqueue(MockResponse().setResponseCode(204))

        val result = client.acknowledgeFinding("RISKY_SERVICE|dev:7|445")

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/security/acknowledged/RISKY_SERVICE%7Cdev%3A7%7C445", request.path)
        assertEquals("Bearer un-token-de-prueba", request.getHeader("Authorization"))
    }

    @Test
    fun `acknowledgeFinding sin nota manda el cuerpo vacio (nota es opcional)`() = runTest {
        server.enqueue(MockResponse().setResponseCode(204))

        client.acknowledgeFinding("RISKY_SERVICE|192.168.0.55|445")

        assertEquals("""{}""", server.takeRequest().body.readUtf8())
    }

    @Test
    fun `acknowledgeFinding con nota la manda en el cuerpo`() = runTest {
        server.enqueue(MockResponse().setResponseCode(204))

        client.acknowledgeFinding("RISKY_SERVICE|192.168.0.55|445", note = "es el RTSP de Frigate")

        assertEquals("""{"note":"es el RTSP de Frigate"}""", server.takeRequest().body.readUtf8())
    }

    @Test
    fun `unacknowledgeFinding manda DELETE con el id url-encoded`() = runTest {
        server.enqueue(MockResponse().setResponseCode(204))

        val result = client.unacknowledgeFinding("RISKY_SERVICE|dev:7|445")

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/security/acknowledged/RISKY_SERVICE%7Cdev%3A7%7C445", request.path)
    }

    // ---- Webhooks (ver el contrato § "Webhooks" y `ui/agent/WebhooksScreen.kt`) ----

    @Test
    fun `webhooks pide la lista con GET y token`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """[{"id":3,"url":"http://homeassistant.local:8123/api/webhook/vigia","types":["NEW_DEVICE"],"lastStatus":200}]""",
            ),
        )

        val result = client.webhooks()

        val webhooks = result.getOrThrow()
        assertEquals(1, webhooks.size)
        assertEquals(3L, webhooks[0].id)
        assertEquals(200, webhooks[0].lastStatus)
        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/webhooks", request.path)
        assertEquals("Bearer un-token-de-prueba", request.getHeader("Authorization"))
    }

    @Test
    fun `addWebhook manda url, tipos y secreto en el cuerpo`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":4,"url":"https://ejemplo.com/webhook","types":["NEW_DEVICE","INTERNET_DOWN"]}""",
            ),
        )

        val result = client.addWebhook("https://ejemplo.com/webhook", listOf("NEW_DEVICE", "INTERNET_DOWN"), "un-secreto")

        val webhook = result.getOrThrow()
        assertEquals(4L, webhook.id)
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/webhooks", request.path)
        val body = request.body.readUtf8()
        assertTrue(body.contains("\"url\":\"https://ejemplo.com/webhook\""))
        assertTrue(body.contains("\"secret\":\"un-secreto\""))
        assertTrue(body.contains("NEW_DEVICE"))
    }

    @Test
    fun `addWebhook sin secreto no manda el campo como texto vacio`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":5,"url":"https://ejemplo.com/webhook","types":[]}"""))

        client.addWebhook("https://ejemplo.com/webhook", emptyList(), null)

        val body = server.takeRequest().body.readUtf8()
        assertFalse(body.contains("\"secret\":\""))
    }

    @Test
    fun `deleteWebhook manda DELETE al id`() = runTest {
        server.enqueue(MockResponse().setResponseCode(204))

        val result = client.deleteWebhook(4)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/webhooks/4", request.path)
    }

    @Test
    fun `testWebhook manda POST y devuelve el status que mando el agente`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"status":200}"""))

        val result = client.testWebhook(4)

        val outcome = result.getOrThrow()
        assertEquals(200, outcome.status)
        assertNull(outcome.error)
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/webhooks/4/test", request.path)
    }

    @Test
    fun `testWebhook con error del agente lo devuelve en el resultado, no como excepcion`() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"error":"conexión rechazada"}"""))

        val result = client.testWebhook(4)

        val outcome = result.getOrThrow()
        assertNull(outcome.status)
        assertEquals("conexión rechazada", outcome.error)
    }
}
