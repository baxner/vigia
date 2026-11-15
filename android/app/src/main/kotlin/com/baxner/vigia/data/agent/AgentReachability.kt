package com.baxner.vigia.data.agent

/**
 * Qué direcciones probar, en qué orden, para alcanzar al agente cuando el
 * teléfono no está en la misma LAN que vio por última vez (ver el contrato
 * § "Alertas v1.1": "`/info` añade `addresses`... para que la app alcance al
 * agente fuera de casa", y el encargo, punto 4: "si la IP de la LAN no
 * responde, prueba las `addresses` de `/info`").
 *
 * Pura — solo arma la lista de candidatos, no abre ninguna conexión — para
 * poder probarla con JUnit plano; quien la use (ver [com.baxner.vigia.work.AlertsSyncWorker]
 * y `AlertsRepository`) intenta cada URL en orden hasta que una conteste.
 */
object AgentReachability {
    /**
     * [linkHost]/[linkPort] son los del vínculo ([AgentLink], guardados al
     * emparejar: casi siempre la IP de la LAN de casa). [savedAddresses] son
     * las `addresses` de `/info` guardadas por [AgentModeController] (ver
     * [AlertsStore.agentAddresses]) — se les asume el mismo puerto que
     * [linkPort] (el agente escucha ese puerto en todas sus interfaces).
     *
     * El host del vínculo va primero (es la ruta más probable y rápida
     * estando en casa); las demás, en el orden que las mandó `/info`, sin
     * repetir el host del vínculo ni direcciones duplicadas entre sí.
     */
    fun candidateBaseUrls(linkHost: String, linkPort: Int, savedAddresses: List<String>): List<String> {
        // HTTPS (ver TLS v1.5): el agente sirve el mismo certificado en
        // todas sus interfaces, así que la huella fijada en el vínculo vale
        // igual para la IP de la LAN que para la de Tailscale.
        val primary = AgentTls.httpsBaseUrl(linkHost, linkPort)
        val fallback = savedAddresses
            .map { it.trim() }
            .filter { it.isNotBlank() && it != linkHost && isLanOrTailscale(it) }
            // Si el host del vínculo no contestó, lo más probable es estar
            // fuera de casa, donde solo llega Tailscale: probarla antes que
            // otras IP privadas del agente (p. ej. la de VirtualBox), que
            // solo harían esperar el timeout. sortedBy es estable.
            .sortedBy { if (isTailscale(it)) 0 else 1 }
            .map { AgentTls.httpsBaseUrl(it, linkPort) }
        return (listOf(primary) + fallback).distinct()
    }

    /**
     * El token solo viaja por la LAN o por Tailscale (cifrado de punta a
     * punta), nunca a una IP pública: las `addresses` vienen de `/info`, que
     * no pide token, así que un falso agente en la LAN podría anunciar una.
     */
    fun isLanOrTailscale(ip: String): Boolean {
        val o = ipv4Octets(ip) ?: return false
        return o[0] == 10 || (o[0] == 172 && o[1] in 16..31) || (o[0] == 192 && o[1] == 168) || isTailscale(ip)
    }

    /** 100.64.0.0/10: el rango CGNAT que usa Tailscale. */
    fun isTailscale(ip: String): Boolean {
        val o = ipv4Octets(ip) ?: return false
        return o[0] == 100 && o[1] in 64..127
    }

    private fun ipv4Octets(ip: String): List<Int>? =
        ip.split('.').map { part -> part.toIntOrNull()?.takeIf { it in 0..255 } ?: return null }
            .takeIf { it.size == 4 }
}
