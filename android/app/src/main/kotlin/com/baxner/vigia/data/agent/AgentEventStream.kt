package com.baxner.vigia.data.agent

import com.baxner.vigia.data.repository.VigiaJson
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable
import kotlinx.serialization.decodeFromString
import okhttp3.Call
import okhttp3.OkHttpClient
import okhttp3.Request

/** Lo que manda `GET /api/v1/stream` (ver el contrato), ya interpretado. */
sealed interface AgentStreamEvent {
    data class ScanFinished(val networkKey: String, val startedAt: String, val finishedAt: String, val devices: Int) : AgentStreamEvent
    data class DeviceChanged(val event: DeviceEventDto, val device: KnownDeviceDto) : AgentStreamEvent
    /** `event: alert` (ver el contrato § "Alertas v1.1" → "Stream"): el [AlertDto] completo, igual que lo da `GET /api/v1/alerts`. */
    data class AlertReceived(val alert: AlertDto) : AgentStreamEvent
}

/** Un bloque SSE ya separado en campos: `event:` (puede faltar) y `data:` (varias líneas se concatenan con `\n`, como pide la especificación). */
data class SseMessage(val event: String?, val data: String)

/**
 * Parsea un bloque de SSE (las líneas entre dos saltos de línea en blanco, ya
 * sin el separador final). Pura — sin red — para poder probarla con JUnit
 * plano. Ignora las líneas de comentario (empiezan con `:`, como el `: ping`
 * que manda el agente cada 25 s para mantener viva la conexión); si el
 * bloque entero es solo eso, devuelve `null`.
 */
fun parseSseBlock(block: String): SseMessage? {
    var event: String? = null
    val data = StringBuilder()
    var sawField = false
    for (rawLine in block.split('\n')) {
        val line = rawLine.trimEnd('\r')
        if (line.isEmpty() || line.startsWith(":")) continue
        val colon = line.indexOf(':')
        val field = if (colon == -1) line else line.substring(0, colon)
        val value = if (colon == -1) "" else line.substring(colon + 1).removePrefix(" ")
        when (field) {
            "event" -> { event = value; sawField = true }
            "data" -> {
                if (data.isNotEmpty()) data.append('\n')
                data.append(value)
                sawField = true
            }
            else -> Unit // "id:", "retry:": el contrato no los usa; se ignoran en vez de romper el parseo.
        }
    }
    return if (sawField) SseMessage(event, data.toString()) else null
}

@Serializable
private data class ScanFinishedPayload(val network: String, val startedAt: String, val finishedAt: String, val devices: Int)

@Serializable
private data class DeviceChangedPayload(val event: DeviceEventDto, val device: KnownDeviceDto)

/** [SseMessage] -> [AgentStreamEvent], o `null` si no es un `event:` que conozcamos (p. ej. uno nuevo de una versión futura del agente) o el JSON no se pudo leer. */
fun SseMessage.toStreamEvent(): AgentStreamEvent? = when (event) {
    "scan" -> runCatching { VigiaJson.decodeFromString<ScanFinishedPayload>(data) }.getOrNull()
        ?.let { AgentStreamEvent.ScanFinished(it.network, it.startedAt, it.finishedAt, it.devices) }
    "device" -> runCatching { VigiaJson.decodeFromString<DeviceChangedPayload>(data) }.getOrNull()
        ?.let { AgentStreamEvent.DeviceChanged(it.event, it.device) }
    "alert" -> runCatching { VigiaJson.decodeFromString<AlertDto>(data) }.getOrNull()
        ?.let { AgentStreamEvent.AlertReceived(it) }
    else -> null
}

/**
 * Lector de `GET /api/v1/stream` con reconexión propia (ver el encargo:
 * "okhttp-sse o lector propio con reconexión" — se eligió lo segundo para no
 * sumar una dependencia más). Mientras se colecte el [Flow] de [events],
 * mantiene la conexión abierta; si se corta (el agente se reinicia, la Wi-Fi
 * falla, el teléfono sale de la red) reintenta con backoff exponencial. Al
 * cancelar la colección se cierra la llamada HTTP en curso (ver [awaitClose]).
 */
class AgentEventStream(
    private val httpClient: OkHttpClient,
    baseUrl: String,
    private val initialBackoffMs: Long = 1_000L,
    private val maxBackoffMs: Long = 30_000L,
    // Último parámetro a propósito (ver el encargo de llamada con lambda al
    // final): se lee en cada intento de conexión, nunca cacheado, para que
    // un token nuevo (o un desvinculado) se note en el próximo reintento.
    private val tokenProvider: () -> String?,
) {
    private val streamUrl = baseUrl.trimEnd('/') + "/api/v1/stream"

    fun events(): Flow<AgentStreamEvent> = callbackFlow {
        var currentCall: Call? = null
        val job = launch(Dispatchers.IO) {
            var backoffMs = initialBackoffMs
            while (isActive) {
                val token = tokenProvider()
                if (token == null) {
                    delay(backoffMs)
                    continue
                }
                val request = Request.Builder()
                    .url(streamUrl)
                    .header("Authorization", "Bearer $token")
                    .header("Accept", "text/event-stream")
                    .build()
                val call = httpClient.newCall(request)
                currentCall = call
                try {
                    call.execute().use { response ->
                        if (!response.isSuccessful) throw IOException("stream HTTP ${response.code}")
                        backoffMs = initialBackoffMs // conexión lograda: la próxima vez que se corte, vuelve a empezar desde el mínimo.
                        val source = response.body?.source() ?: throw IOException("el stream no trajo cuerpo")
                        val block = StringBuilder()
                        while (isActive) {
                            val line = source.readUtf8Line() ?: break // el agente cerró la conexión de su lado.
                            if (line.isEmpty()) {
                                if (block.isNotEmpty()) {
                                    parseSseBlock(block.toString())?.toStreamEvent()?.let { trySend(it) }
                                    block.clear()
                                }
                            } else {
                                if (block.isNotEmpty()) block.append('\n')
                                block.append(line)
                            }
                        }
                    }
                } catch (_: IOException) {
                    // Se perdió la conexión: se reintenta abajo con backoff. No es un
                    // error que deba tumbar el Flow (ver el encargo: "vuelve a modo
                    // local sin romper nada" la decide AgentModeController; este lector
                    // solo se queda insistiendo).
                }
                if (!isActive) break
                delay(backoffMs)
                backoffMs = (backoffMs * 2).coerceAtMost(maxBackoffMs)
            }
        }
        awaitClose {
            currentCall?.cancel()
            job.cancel()
        }
    }.flowOn(Dispatchers.IO)
}
