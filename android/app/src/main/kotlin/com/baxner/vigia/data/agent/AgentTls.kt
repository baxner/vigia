package com.baxner.vigia.data.agent

import android.annotation.SuppressLint
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLSocketFactory
import javax.net.ssl.X509TrustManager
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.OkHttpClient

/**
 * TLS v1.5 (ver `docs/agente-api.md`, "TLS (v1.5)"): el agente sirve su API
 * con un certificado propio autofirmado, fijado por huella al vincular —
 * nunca se valida contra una CA ni se compara el hostname (es una IP de la
 * LAN o de Tailscale, no un nombre). Este archivo junta lo que necesitan
 * [AgentModeController.httpClientFor]/[AgentModeController.apiClientFor] y
 * [AlertsRepository]: las URL por HTTPS/HTTP de transición, el
 * `TrustManager` fijado por huella, el de "confiar la primera vez" (solo
 * para vincularse) y la excepción con la que se distingue "cambió el
 * certificado" de cualquier otro fallo de red.
 */
object AgentTls {
    /** Igual que [agentBaseUrl] (ver `AgentDiscovery.kt`) pero por HTTPS: el puerto que anuncia mDNS y guarda el vínculo. */
    fun httpsBaseUrl(host: String, port: Int): String =
        HttpUrl.Builder().scheme("https").host(host.substringBefore('%')).port(port).build().toString().trimEnd('/')

    /**
     * El HTTP sin cifrar de transición (ver el contrato, "-http-legacy"): el
     * agente lo acepta en el MISMO puerto que HTTPS (distingue por el primer
     * byte). Solo lo usa la migración de un vínculo de antes de esta fase
     * (ver [migrateLegacyLinkIfNeeded]) — nunca una llamada ya autenticada.
     */
    fun legacyHttpBaseUrl(host: String, port: Int): String = agentBaseUrl(host, port)

    /** SHA-256 del certificado (su DER), en hex minúsculas — mismo formato que el "certSha256" del contrato. */
    fun fingerprintOf(cert: X509Certificate): String =
        MessageDigest.getInstance("SHA-256").digest(cert.encoded).joinToString("") { "%02x".format(it) }

    /**
     * Normaliza una huella SHA-256 (p. ej. de TXT `cert` en mDNS o `certSha256`
     * de `/info`): quita prefijos `sha256/` o `sha256:`, dos puntos y espacios,
     * y pasa a minúsculas. Devuelve `null` si queda vacío.
     */
    fun normalizeFingerprint(raw: String?): String? {
        val cleaned = raw?.trim()
            ?.removePrefix("sha256/")
            ?.removePrefix("sha256:")
            ?.replace(":", "")
            ?.replace(" ", "")
            ?.lowercase()
        return cleaned?.takeIf { it.isNotEmpty() }
    }

    /**
     * `TrustManager` que acepta **solo** el certificado cuya huella SHA-256
     * es [expectedFingerprintHex] (ver el contrato: "fijado al vincular").
     * Sin hostname: eso lo decide el `HostnameVerifier` de quien construye
     * el cliente (ver [AgentModeController.httpClientFor]), no este
     * TrustManager. Si el certificado presentado no coincide,
     * [X509TrustManager.checkServerTrusted] lanza [CertPinMismatchException]
     * — es lo que [isCertPinMismatch] busca en la cadena de causas para
     * distinguir "cambió el certificado" de cualquier otro error de red.
     */
    @SuppressLint("CustomX509TrustManager")
    fun pinnedSocketFactory(expectedFingerprintHex: String): Pair<SSLSocketFactory, X509TrustManager> {
        val expected = normalizeFingerprint(expectedFingerprintHex) ?: expectedFingerprintHex
        // Se valida explícitamente la huella SHA-256 del certificado hoja autofirmado.
        val trustManager = object : X509TrustManager {
            override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String) = Unit

            override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String) {
                val leaf = chain.firstOrNull() ?: throw CertPinMismatchException("el agente no mandó ningún certificado")
                val actual = fingerprintOf(leaf)
                if (!actual.equals(expected, ignoreCase = true)) {
                    throw CertPinMismatchException("huella del certificado = $actual, se esperaba $expected")
                }
            }

            override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
        }
        return socketFactoryFor(trustManager) to trustManager
    }

    /**
     * Confía en lo que presente el agente sin comparar ninguna huella —
     * solo para el momento de vincularse o previsualizar (ver
     * [AgentModeController.pair]/[AgentModeController.previewAgent]):
     * todavía no hay nada guardado con qué compararlo, y el usuario está
     * tecleando a mano el código de 6 dígitos que vio en la pantalla del
     * agente — el mismo nivel de confianza que tenía la vinculación por
     * HTTP antes de esta fase. La huella de verdad llega en el cuerpo de la
     * respuesta (`certSha256`, ver el contrato) y es lo que se guarda en el
     * vínculo para todas las llamadas de ahí en adelante.
     */
    @SuppressLint("CustomX509TrustManager")
    fun trustOnFirstUseSocketFactory(): Pair<SSLSocketFactory, X509TrustManager> {
        // Solo para vinculación TOFU inicial con código de 6 dígitos; certSha256 se fija inmediatamente al emparejar.
        val trustManager = object : X509TrustManager {
            override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String) = Unit
            override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String) = Unit
            override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
        }
        return socketFactoryFor(trustManager) to trustManager
    }

    private fun socketFactoryFor(trustManager: X509TrustManager): SSLSocketFactory {
        val context = SSLContext.getInstance("TLS")
        context.init(null, arrayOf(trustManager), null)
        return context.socketFactory
    }
}

/** Ver [AgentTls.pinnedSocketFactory]: el certificado presentado no es el que se fijó al vincular (o al migrar). */
class CertPinMismatchException(message: String) : CertificateException(message)

/** Busca [CertPinMismatchException] en toda la cadena de causas: OkHttp/JSSE envuelven lo que lanza el TrustManager en un SSLHandshakeException. */
fun Throwable.isCertPinMismatch(): Boolean = generateSequence(this) { it.cause }.any { it is CertPinMismatchException }

/**
 * Lee la huella del certificado para un vínculo de antes de TLS v1.5 (ver
 * `docs/agente-api.md`, "TLS (v1.5)" → "La app"): `GET /info` por HTTP, la
 * vía vieja, mientras dure `-http-legacy`. Si no contesta, no significa que
 * "cambió el certificado" — significa "todavía no migrado, o inalcanzable
 * ahora": se devuelve `null` y quien llama lo trata como cualquier agente
 * inalcanzable (se reintenta en la próxima evaluación). Si contesta, guarda
 * la huella en [linkStore] y devuelve el vínculo ya migrado.
 *
 * Suspend y sin estado propio — ni depende de [CurrentNetwork][com.baxner.vigia.data.network.CurrentNetwork]
 * ni de una sesión activa — para que la comparta tanto
 * [AgentModeController.evaluate] (que sí tiene una red atada) como
 * [AlertsRepository] (que no): mismo [AgentLinkStore], una sola huella que
 * fijar la primera vez que cualquiera de los dos la necesite.
 */
suspend fun migrateLegacyLinkIfNeeded(linkStore: AgentLinkStore, link: AgentLink, httpClient: OkHttpClient): AgentLink? {
    if (link.certSha256 != null) return link
    val legacyClient = AgentApiClient(httpClient, AgentTls.legacyHttpBaseUrl(link.host, link.port)) { null }
    val fingerprint = legacyClient.info().getOrNull()?.certSha256?.takeIf { it.isNotBlank() } ?: return null
    val migrated = link.copy(certSha256 = fingerprint)
    linkStore.save(migrated)
    return migrated
}

/** [this] con el `TrustManager` fijado a [fingerprint] (ver [AgentTls.pinnedSocketFactory]): mismos timeouts, sin hostname (es una IP de la LAN o de Tailscale). */
fun OkHttpClient.pinnedTo(fingerprint: String): OkHttpClient {
    val (sslSocketFactory, trustManager) = AgentTls.pinnedSocketFactory(fingerprint)
    return newBuilder()
        .sslSocketFactory(sslSocketFactory, trustManager)
        .hostnameVerifier { _, _ -> true }
        .build()
}

/**
 * Como [withPinnedReachableAgent] pero devuelve también el [AgentApiClient]
 * que contestó (no solo el resultado de [probe]): para reusarlo el resto de
 * una sesión completa — API más stream SSE, ver
 * [AgentModeController.activateRemote] — en vez de repetir la prueba de
 * "LAN y luego Tailscale" en cada llamada (ver el encargo, fase 8: "reutiliza
 * withPinnedReachableAgent/AgentReachability en vez de duplicar la lógica").
 * [withPinnedReachableAgent] es ahora un envoltorio fino de esta función.
 *
 * Intenta [probe] contra cada dirección candidata del agente (LAN del
 * vínculo, luego Tailscale, ver [AgentReachability]) hasta que una tenga
 * éxito, todas por HTTPS fijadas a la misma huella (el agente sirve el mismo
 * certificado en todas sus interfaces). Si el vínculo todavía no tiene
 * huella, la toma antes por la vía vieja ([migrateLegacyLinkIfNeeded]).
 */
suspend fun <T> resolvePinnedReachableAgent(
    linkStore: AgentLinkStore,
    savedAddresses: List<String>,
    httpClient: OkHttpClient,
    probe: suspend (AgentApiClient) -> Result<T>,
): Result<Pair<AgentApiClient, T>> {
    val storedLink = linkStore.get() ?: return Result.failure(AgentApiException(401, "sin vínculo: el vínculo no está activo"))
    val link = migrateLegacyLinkIfNeeded(linkStore, storedLink, httpClient)
        ?: return Result.failure(AgentApiException(404, "el agente todavía no contesta (sin huella TLS fijada)"))
    val pinnedClient = httpClient.pinnedTo(link.certSha256!!)
    val candidates = AgentReachability.candidateBaseUrls(link.host, link.port, savedAddresses)
    var lastFailure: Throwable = AgentApiException(404, "ninguna de ${candidates.size} dirección(es) del agente contestó")
    for (base in candidates) {
        val client = AgentApiClient(pinnedClient, base) { linkStore.get()?.token }
        val result = probe(client)
        if (result.isSuccess) return result.map { client to it }
        result.exceptionOrNull()?.let { lastFailure = it.unlessForeignLanCert(base) }
    }
    return Result.failure(lastFailure)
}

/**
 * Intenta [block] contra cada dirección candidata del agente (LAN del
 * vínculo, luego Tailscale, ver [AgentReachability]) hasta que una tenga
 * éxito. Compartido por [AlertsRepository] y [WebhooksRepository]: lo que
 * funciona fuera de casa sin pasar por [AgentModeController] — administrar
 * alertas/webhooks no necesita quedarse con el [AgentApiClient] que contestó
 * (cada llamada es independiente, ver [resolvePinnedReachableAgent] si se
 * necesita una sesión).
 */
suspend fun <T> withPinnedReachableAgent(
    linkStore: AgentLinkStore,
    savedAddresses: List<String>,
    httpClient: OkHttpClient,
    block: suspend (AgentApiClient) -> Result<T>,
): Result<T> = resolvePinnedReachableAgent(linkStore, savedAddresses, httpClient, block).map { it.second }

/**
 * Una huella distinta en la IP de la LAN del vínculo no prueba que el agente
 * haya cambiado de certificado: en otra red con la misma numeración (la casa
 * de alguien, otro 192.168.0.x) en esa IP puede contestar cualquier equipo
 * con su propio certificado. Se trata como "no contesta" — el token nunca
 * llegó a mandarse, el handshake se cortó antes. Solo una dirección de
 * Tailscale (que solo alcanza a equipos de la tailnet) da un "cambió el
 * certificado" confiable.
 */
internal fun Throwable.unlessForeignLanCert(baseUrl: String): Throwable {
    if (!isCertPinMismatch()) return this
    val host = baseUrl.toHttpUrlOrNull()?.host ?: return this
    if (AgentReachability.isTailscale(host)) return this
    return AgentApiException(404, "en $host contestó un equipo con otro certificado (¿otra red con la misma numeración?)")
}
