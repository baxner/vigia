package com.baxner.vigia.data.agent

import android.content.Context
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient

/**
 * Administración de webhooks desde la app (ver el contrato § "Webhooks" y el
 * encargo de la fase 8, "Webhooks desde la app"): listar, agregar, borrar y
 * "Probar". Mismo patrón que [AlertsRepository.withReachableAgent]: prueba
 * primero la LAN del vínculo y, si no contesta, las direcciones de Tailscale
 * guardadas (ver [AgentReachability]) — administrar webhooks debe poder
 * hacerse aunque el teléfono esté fuera de casa, igual que leer o marcar
 * alertas, así que no depende de "Red monitoreada" ([AgentModeController]).
 *
 * Sin estado propio que cachear entre pantallas (a diferencia de
 * [AlertsRepository]): la lista de webhooks no se necesita fuera de la
 * pantalla de Ajustes, así que cada llamada va directo a la red.
 */
class WebhooksRepository(
    context: Context,
    private val linkStore: AgentLinkStore = EncryptedAgentLinkStore(context),
    private val alertsStore: AlertsStore = SharedPrefsAlertsStore(context),
) {
    private val httpClient = OkHttpClient.Builder()
        .connectTimeout(ConnectTimeoutSeconds, TimeUnit.SECONDS)
        .readTimeout(ReadTimeoutSeconds, TimeUnit.SECONDS)
        .build()

    suspend fun list(): Result<List<WebhookDto>> = withReachableAgent { it.webhooks() }

    suspend fun add(url: String, types: List<String>, secret: String?): Result<WebhookDto> =
        withReachableAgent { it.addWebhook(url, types, secret) }

    suspend fun delete(id: Long): Result<Unit> = withReachableAgent { it.deleteWebhook(id) }

    suspend fun test(id: Long): Result<WebhookTestResultDto> = withReachableAgent { it.testWebhook(id) }

    /** Ver [withPinnedReachableAgent]: igual criterio que [AlertsRepository], con la huella TLS fijada. */
    private suspend fun <T> withReachableAgent(block: suspend (AgentApiClient) -> Result<T>): Result<T> = withContext(Dispatchers.IO) {
        withPinnedReachableAgent(linkStore, alertsStore.agentAddresses(), httpClient, block)
    }

    private companion object {
        const val ConnectTimeoutSeconds = 4L
        const val ReadTimeoutSeconds = 8L
    }
}
