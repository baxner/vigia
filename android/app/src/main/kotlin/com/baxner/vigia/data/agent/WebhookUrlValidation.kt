package com.baxner.vigia.data.agent

import java.net.Inet4Address
import java.net.InetAddress
import java.net.URI

/**
 * Valida la URL que se escribe en "Agregar webhook" (ver `ui/agent/WebhooksScreen.kt`
 * y el contrato § "Webhooks": "Solo `http`/`https`"). Pura — sin red, sin
 * Android — para poder probarla con JUnit plano; además de exigir `http`/`https`
 * y host válido, rechaza loopback (`localhost`, `127.0.0.1`, `::1`), `0.0.0.0`
 * y direcciones de enlace local (`169.254.*`, `fe80:*`) para evitar SSRF contra
 * el propio dispositivo o metadatos locales.
 */
fun isValidWebhookUrl(raw: String): Boolean {
    val trimmed = raw.trim()
    if (trimmed.isEmpty()) return false
    val uri = runCatching { URI(trimmed) }.getOrNull() ?: return false
    val scheme = uri.scheme?.lowercase() ?: return false
    if (scheme != "http" && scheme != "https") return false
    val host = uri.host?.takeIf { it.isNotBlank() } ?: return false
    return !isDisallowedWebhookHost(host)
}

private val NumericIpLikeRegex = Regex("""^[\d.]+$""")
private val HexIpRegex = Regex("""^0x[0-9a-f]+$""")
private val Ipv4DottedRegex = Regex("""^\d{1,3}(\.\d{1,3}){3}$""")

private fun isDisallowedWebhookHost(rawHost: String): Boolean {
    val host = rawHost.trim()
        .removePrefix("[")
        .removeSuffix("]")
        .substringBefore('%')
        .trimEnd('.')
        .lowercase()
    if (host.isEmpty()) return true
    if (
        host == "localhost" ||
        host.endsWith(".localhost") ||
        host == "ip6-localhost" ||
        host == "ip6-loopback"
    ) {
        return true
    }

    // Literal IPv6 o numérico IPv4: se parsea localmente sin disparar DNS.
    if (host.contains(':')) {
        val addr = runCatching { InetAddress.getByName("[$host]") }.getOrNull() ?: return true
        return isForbiddenAddress(addr)
    }
    if (HexIpRegex.matches(host)) return true
    if (NumericIpLikeRegex.matches(host)) {
        if (!Ipv4DottedRegex.matches(host)) return true
        val octets = host.split('.').mapNotNull { it.toIntOrNull() }
        if (octets.size != 4 || octets.any { it !in 0..255 }) return true
        val addr = runCatching { InetAddress.getByName(host) }.getOrNull() ?: return true
        return isForbiddenAddress(addr)
    }
    return false
}

private fun isForbiddenAddress(addr: InetAddress): Boolean {
    if (addr.isLoopbackAddress || addr.isAnyLocalAddress || addr.isLinkLocalAddress) return true
    if (addr is Inet4Address && addr.address[0].toInt() == 0) return true
    return false
}
