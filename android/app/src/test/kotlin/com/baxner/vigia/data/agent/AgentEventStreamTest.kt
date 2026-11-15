package com.baxner.vigia.data.agent

import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * El stream del agente (ver `docs/agente-api.md` § Stream): el parser de
 * bloques SSE es puro (sin red, ver los primeros tests); [AgentEventStream]
 * en sí se prueba contra MockWebServer, incluida la reconexión cuando el
 * agente cierra la conexión de su lado.
 */
class AgentEventStreamTest {

    // ---- parseSseBlock / toStreamEvent: puro ----

    @Test
    fun `evento con event y data de una linea`() {
        val message = parseSseBlock("event: scan\ndata: {\"a\":1}")
        assertEquals("scan", message?.event)
        assertEquals("{\"a\":1}", message?.data)
    }

    @Test
    fun `varias lineas data se concatenan con salto de linea`() {
        val message = parseSseBlock("event: device\ndata: linea1\ndata: linea2")
        assertEquals("linea1\nlinea2", message?.data)
    }

    @Test
    fun `un bloque que es solo el ping comentario no es un mensaje`() {
        assertNull(parseSseBlock(": ping"))
    }

    @Test
    fun `bloque vacio no es un mensaje`() {
        assertNull(parseSseBlock(""))
    }

    @Test
    fun `data sin event sigue siendo un mensaje (event null)`() {
        val message = parseSseBlock("data: {\"a\":1}")
        assertNull(message?.event)
        assertEquals("{\"a\":1}", message?.data)
    }

    @Test
    fun `toStreamEvent de un scan valido`() {
        val message = SseMessage(
            event = "scan",
            data = """{"network":"192.168.0.1|192.168.0.0/24","startedAt":"2026-10-06T22:00:00Z","finishedAt":"2026-10-06T22:00:05Z","devices":5}""",
        )
        val event = message.toStreamEvent() as? AgentStreamEvent.ScanFinished
        assertEquals("192.168.0.1|192.168.0.0/24", event?.networkKey)
        assertEquals(5, event?.devices)
    }

    @Test
    fun `toStreamEvent de un device valido`() {
        val message = SseMessage(
            event = "device",
            data = """{"event":{"deviceId":1,"type":"CAME_ONLINE","at":"2026-10-06T22:05:00Z"},"device":{"id":1,"network":"k","lastIp":"192.168.0.10","firstSeen":"2026-10-06T22:00:00Z","lastSeen":"2026-10-06T22:05:00Z","online":true,"last":{}}}""",
        )
        val event = message.toStreamEvent() as? AgentStreamEvent.DeviceChanged
        assertEquals(1L, event?.device?.id)
        assertEquals("CAME_ONLINE", event?.event?.type)
    }

    @Test
    fun `evento desconocido no revienta, devuelve null`() {
        assertNull(SseMessage(event = "futuro", data = "{}").toStreamEvent())
    }

    @Test
    fun `toStreamEvent de un alert valido`() {
        val message = SseMessage(
            event = "alert",
            data = """{"id":57,"type":"NEW_DEVICE","severity":"high","at":"2026-10-07T07:10:00Z","title":"Equipo nuevo en tu red","body":"Telefono-Invitado (192.168.0.24) se conectó por primera vez.","deviceId":31,"network":"k","read":false}""",
        )
        val event = message.toStreamEvent() as? AgentStreamEvent.AlertReceived
        assertEquals(57L, event?.alert?.id)
        assertEquals("NEW_DEVICE", event?.alert?.type)
        assertEquals(31L, event?.alert?.deviceId)
    }

    @Test
    fun `toStreamEvent de un alert de tipo AGENT_ sin deviceId`() {
        val message = SseMessage(
            event = "alert",
            data = """{"id":58,"type":"AGENT_BLIND","severity":"medium","at":"2026-10-07T07:10:00Z","title":"t","body":"b","read":false}""",
        )
        val event = message.toStreamEvent() as? AgentStreamEvent.AlertReceived
        assertNull(event?.alert?.deviceId)
    }

    @Test
    fun `json malformado no revienta, devuelve null`() {
        assertNull(SseMessage(event = "scan", data = "no es json").toStreamEvent())
    }

    // ---- AgentEventStream: MockWebServer, con reconexión ----

    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    // runBlocking (no runTest): esto hace I/O real contra MockWebServer y
    // espera backoffs reales de pocos ms en Dispatchers.IO; con runTest, el
    // reloj virtual del scheduler de pruebas adelanta el withTimeout de
    // golpe sin dejar correr ese hilo real, y el test revienta por timeout
    // sin haber esperado de verdad.
    fun `recibe eventos y reconecta cuando el agente cierra la conexion`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "text/event-stream")
                .setBody(
                    "event: scan\n" +
                        "data: {\"network\":\"k\",\"startedAt\":\"2026-10-06T22:00:00Z\",\"finishedAt\":\"2026-10-06T22:00:05Z\",\"devices\":3}\n\n",
                ),
        )
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "text/event-stream")
                .setBody(
                    "event: device\n" +
                        "data: {\"event\":{\"deviceId\":9,\"type\":\"CAME_ONLINE\",\"at\":\"2026-10-06T22:05:00Z\"}," +
                        "\"device\":{\"id\":9,\"network\":\"k\",\"lastIp\":\"192.168.0.9\",\"firstSeen\":\"2026-10-06T22:00:00Z\"," +
                        "\"lastSeen\":\"2026-10-06T22:05:00Z\",\"online\":true,\"last\":{}}}\n\n",
                ),
        )

        val stream = AgentEventStream(
            httpClient = OkHttpClient(),
            baseUrl = server.url("/").toString(),
            initialBackoffMs = 10L,
            maxBackoffMs = 50L,
        ) { "un-token" }

        val events = withTimeout(10_000) { stream.events().take(2).toList() }

        val scan = events[0] as AgentStreamEvent.ScanFinished
        val device = events[1] as AgentStreamEvent.DeviceChanged
        assertEquals(3, scan.devices)
        assertEquals(9L, device.device.id)

        // Al menos dos conexiones: la primera se cerró sola (fin del cuerpo) y
        // el lector reconectó solo. "Al menos": tras la segunda, el lector
        // puede reconectar otra vez (backoff de 10 ms) antes de que take(2) lo
        // corte; con la máquina cargada pasaba y el test fallaba al azar.
        assertTrue("conexiones: ${server.requestCount}", server.requestCount >= 2)
        val firstRequest = server.takeRequest()
        assertEquals("Bearer un-token", firstRequest.getHeader("Authorization"))
        assertTrue(firstRequest.path.orEmpty().endsWith("/api/v1/stream"))
    }

    @Test
    fun `sin token espera con backoff en vez de fallar, y conecta cuando aparece`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "text/event-stream")
                .setBody("event: scan\ndata: {\"network\":\"k\",\"startedAt\":\"a\",\"finishedAt\":\"b\",\"devices\":1}\n\n"),
        )

        // Las dos primeras consultas del token todavía no hay vínculo activo (null); la tercera ya sí.
        var calls = 0
        val stream = AgentEventStream(
            httpClient = OkHttpClient(),
            baseUrl = server.url("/").toString(),
            initialBackoffMs = 10L,
            maxBackoffMs = 20L,
        ) { calls++; if (calls < 3) null else "token" }

        val events = withTimeout(10_000) { stream.events().take(1).toList() }

        assertEquals(1, (events.single() as AgentStreamEvent.ScanFinished).devices)
        assertTrue(calls >= 3)
    }
}
