package com.baxner.vigia.data.agent

import com.baxner.vigia.data.repository.VigiaJson
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody

private val JsonMediaType = "application/json; charset=utf-8".toMediaType()
private val EmptyJsonBody: RequestBody = "".toRequestBody(JsonMediaType)

/** El agente contestó un error HTTP (4xx/5xx) con `{"error": "…"}` (ver el contrato), o no hay token para una llamada que lo necesita. */
class AgentApiException(val status: Int, override val message: String) : IOException(message)

/**
 * Cliente HTTP del agente 24/7 (ver `docs/agente-api.md`). El token siempre
 * va en la cabecera `Authorization: Bearer <token>`, nunca en la URL (ver
 * [authedRequest]). [httpClient] debe venir ya atado a la red Wi-Fi correcta —
 * [com.baxner.vigia.data.network.NetworkInfoProvider.bindProcessTo] o
 * `Network.socketFactory`, igual que el escaneo — para que con una VPN
 * activa (Tailscale) la llamada salga por la LAN y no por el túnel; eso lo
 * decide quien construye el [OkHttpClient] (ver `AgentModeController`), no
 * esta clase. Las pruebas le pasan un cliente sin atar, apuntado a
 * MockWebServer.
 *
 * Cada método recibe a [execute]/[executeNoContent] una función que recién
 * *construye* el [Request] (no uno ya armado): así, si [authedRequest] falla
 * por falta de token, ese error cae dentro del mismo `runCatching` que
 * atrapa los errores de red, y el método sigue devolviendo un [Result] en
 * vez de lanzar la excepción a quien llama.
 */
class AgentApiClient(
    private val httpClient: OkHttpClient,
    baseUrl: String,
    /** Token vigente, o null si todavía no hay vínculo (p. ej. durante el `pair`). Se lee en cada llamada: si el usuario desvincula a medio escaneo, la siguiente llamada ya no manda un token viejo. */
    private val tokenProvider: () -> String?,
) {
    /** La URL con la que se armó este cliente (ver [resolvePinnedReachableAgent]/[AgentModeController]: la sesión remota la reusa para el stream SSE, la misma dirección que contestó la prueba). */
    val baseUrl: String = baseUrl.trimEnd('/')

    private fun url(path: String): String = baseUrl + path

    private fun authedRequest(): Request.Builder {
        val token = tokenProvider() ?: throw AgentApiException(401, "sin token: el vínculo no está activo")
        return Request.Builder().header("Authorization", "Bearer $token")
    }

    /** `GET /api/v1/info`: sin token (ver el contrato), para mostrar a qué agente te vas a vincular. */
    suspend fun info(): Result<AgentInfoDto> =
        execute { Request.Builder().url(url("/api/v1/info")).get().build() }

    /** `POST /api/v1/pair`: sin token. */
    suspend fun pair(code: String, client: String): Result<PairResponseDto> = execute {
        val body = VigiaJson.encodeToString(PairRequest(code = code, client = client)).toRequestBody(JsonMediaType)
        Request.Builder().url(url("/api/v1/pair")).post(body).build()
    }

    suspend fun networks(): Result<List<AgentNetworkDto>> =
        execute { authedRequest().url(url("/api/v1/networks")).get().build() }

    /** [networkKey] se manda URL-encoded (ver el contrato: "`{key}` de red va URL-encoded"). */
    suspend fun devicesForNetwork(networkKey: String): Result<List<KnownDeviceDto>> =
        execute { authedRequest().url(url("/api/v1/networks/${encodePathSegment(networkKey)}/devices")).get().build() }

    suspend fun device(id: Long): Result<KnownDeviceDto> =
        execute { authedRequest().url(url("/api/v1/devices/$id")).get().build() }

    suspend fun events(id: Long, limit: Int = 200): Result<List<DeviceEventDto>> =
        execute { authedRequest().url(url("/api/v1/devices/$id/events?limit=$limit")).get().build() }

    suspend fun patchCustomization(id: Long, patch: DeviceCustomizationPatch): Result<KnownDeviceDto> = execute {
        val body = patch.toJsonObject().toString().toRequestBody(JsonMediaType)
        authedRequest().url(url("/api/v1/devices/$id")).patch(body).build()
    }

    suspend fun forget(id: Long): Result<Unit> =
        executeNoContent { authedRequest().url(url("/api/v1/devices/$id")).delete().build() }

    /** `POST /api/v1/scan`: `202 {"started": true}`, o `409` si ya hay uno en curso (ver [AgentApiException.status]). */
    suspend fun startScan(): Result<Boolean> =
        execute<ScanStartedDto> { authedRequest().url(url("/api/v1/scan")).post(EmptyJsonBody).build() }
            .map { it.started }

    // ---- Alertas (v1.1, ver el contrato § "Alertas") ----

    /** `GET /api/v1/alerts?after=<id>&limit=<limit>`: de la más vieja a la más nueva, para ponerse al día (ver el contrato). */
    suspend fun alerts(after: Long = 0, limit: Int = 100): Result<List<AlertDto>> =
        execute { authedRequest().url(url("/api/v1/alerts?after=$after&limit=$limit")).get().build() }

    /** `POST /api/v1/alerts/read {"upTo": <id>}` → `204`: marca leídas todas las alertas con id <= [upTo]. */
    suspend fun markRead(upTo: Long): Result<Unit> = executeNoContent {
        val body = VigiaJson.encodeToString(AlertsReadRequest(upTo)).toRequestBody(JsonMediaType)
        authedRequest().url(url("/api/v1/alerts/read")).post(body).build()
    }

    suspend fun alertSettings(): Result<AlertSettingsDto> =
        execute { authedRequest().url(url("/api/v1/alert-settings")).get().build() }

    /** `PUT /api/v1/alert-settings`: reemplaza por completo (ver el contrato: "reemplazar"). */
    suspend fun putAlertSettings(settings: AlertSettingsDto): Result<AlertSettingsDto> = execute {
        val body = VigiaJson.encodeToString(settings).toRequestBody(JsonMediaType)
        authedRequest().url(url("/api/v1/alert-settings")).put(body).build()
    }

    // ---- Seguridad (v1.2, ver el contrato § "Seguridad") ----

    /** `GET /api/v1/security`: informe calculado al pedirlo, con lo último de cada escaneo (ver el contrato § "Seguridad" → "Informe"). */
    suspend fun security(): Result<SecurityReport> =
        execute { authedRequest().url(url("/api/v1/security")).get().build() }

    // ---- Hallazgos reconocidos (v1.3, ver el contrato § "Hallazgos reconocidos") ----

    /**
     * `PUT /api/v1/security/acknowledged/{findingId}` ([findingId] va
     * URL-encoded, igual que [devicesForNetwork]): "Ya lo sé, es a
     * propósito". `204` sin cuerpo de vuelta.
     */
    suspend fun acknowledgeFinding(findingId: String, note: String? = null): Result<Unit> = executeNoContent {
        val body = VigiaJson.encodeToString(AcknowledgeFindingRequest(note)).toRequestBody(JsonMediaType)
        authedRequest().url(url("/api/v1/security/acknowledged/${encodePathSegment(findingId)}")).put(body).build()
    }

    /** `DELETE /api/v1/security/acknowledged/{findingId}`: "Deshacer". */
    suspend fun unacknowledgeFinding(findingId: String): Result<Unit> =
        executeNoContent { authedRequest().url(url("/api/v1/security/acknowledged/${encodePathSegment(findingId)}")).delete().build() }

    // ---- Internet (v1.4, ver el contrato § "Internet") ----

    /** `GET /api/v1/internet`: el estado calculado por el agente, con lo último de cada medición. */
    suspend fun internet(): Result<InternetStatusDto> =
        execute { authedRequest().url(url("/api/v1/internet")).get().build() }

    /** `GET /api/v1/internet/series?range=24h|7d` (ver el contrato: por minuto o por hora, `null` donde no hubo dato). */
    suspend fun internetSeries(range: InternetSeriesRange): Result<List<InternetSeriesPointDto>> =
        execute { authedRequest().url(url("/api/v1/internet/series?range=${range.wireValue}")).get().build() }

    /**
     * `POST /api/v1/internet/speedtest`: `202 {"started": true}`, o `409` si
     * ya hay una corriendo (ver [AgentApiException.status] — la UI lo
     * muestra como "Ya hay una prueba corriendo", ver `ui/internet/`).
     */
    suspend fun startSpeedTest(): Result<Boolean> =
        execute<ScanStartedDto> { authedRequest().url(url("/api/v1/internet/speedtest")).post(EmptyJsonBody).build() }
            .map { it.started }

    suspend fun internetSettings(): Result<InternetSettingsDto> =
        execute { authedRequest().url(url("/api/v1/internet/settings")).get().build() }

    /** `PUT /api/v1/internet/settings`: reemplaza por completo (mismo trato que [putAlertSettings]). */
    suspend fun putInternetSettings(settings: InternetSettingsDto): Result<InternetSettingsDto> = execute {
        val body = VigiaJson.encodeToString(settings).toRequestBody(JsonMediaType)
        authedRequest().url(url("/api/v1/internet/settings")).put(body).build()
    }

    // ---- Webhooks (ver el contrato § "Alertas" → "Webhooks"), administrados
    // desde Ajustes del agente → "Webhooks" (ver `ui/agent/WebhooksScreen.kt`) ----

    suspend fun webhooks(): Result<List<WebhookDto>> =
        execute { authedRequest().url(url("/api/v1/webhooks")).get().build() }

    /** `POST /api/v1/webhooks`: [secret] opcional, nunca vuelve en la respuesta (ver el contrato). */
    suspend fun addWebhook(urlToNotify: String, types: List<String> = emptyList(), secret: String? = null): Result<WebhookDto> = execute {
        val body = VigiaJson.encodeToString(AddWebhookRequest(url = urlToNotify, types = types, secret = secret)).toRequestBody(JsonMediaType)
        authedRequest().url(url("/api/v1/webhooks")).post(body).build()
    }

    suspend fun deleteWebhook(id: Long): Result<Unit> =
        executeNoContent { authedRequest().url(url("/api/v1/webhooks/$id")).delete().build() }

    /**
     * `POST /api/v1/webhooks/{id}/test`: el agente contesta `200` con
     * `{"status"}` o `{"error"}` tal cual (nunca un error HTTP por esto, ver
     * [WebhookTestResultDto]) — "Probar" en la UI muestra ese resultado.
     */
    suspend fun testWebhook(id: Long): Result<WebhookTestResultDto> =
        execute { authedRequest().url(url("/api/v1/webhooks/$id/test")).post(EmptyJsonBody).build() }

    private suspend inline fun <reified T> execute(crossinline buildRequest: () -> Request): Result<T> =
        withContext(Dispatchers.IO) {
            runCatching {
                val request = buildRequest()
                httpClient.newCall(request).execute().use { response ->
                    val text = response.body?.string().orEmpty()
                    if (!response.isSuccessful) throw response.toApiException(text)
                    VigiaJson.decodeFromString<T>(text)
                }
            }
        }

    private suspend fun executeNoContent(buildRequest: () -> Request): Result<Unit> = withContext(Dispatchers.IO) {
        runCatching {
            val request = buildRequest()
            httpClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) throw response.toApiException(response.body?.string().orEmpty())
            }
        }
    }

    private fun okhttp3.Response.toApiException(body: String): AgentApiException {
        val message = runCatching { VigiaJson.decodeFromString<ApiErrorBody>(body).error }
            .getOrNull()
            ?.takeIf { it.isNotBlank() }
            ?: "error HTTP $code"
        return AgentApiException(code, message)
    }
}

/** Un segmento de ruta URL-encoded (el `{key}` de red trae "|" y "/", ver [NetworkKey][com.baxner.vigia.data.history.NetworkKey]). */
internal fun encodePathSegment(segment: String): String =
    java.net.URLEncoder.encode(segment, "UTF-8").replace("+", "%20")
