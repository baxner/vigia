package com.baxner.vigia.data.agent

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Qué direcciones probar y en qué orden (ver el encargo, punto 4: "si la IP
 * de la LAN no responde, prueba las `addresses` de `/info`"). Pura: ninguna
 * de estas pruebas abre una conexión real.
 */
class AgentReachabilityTest {

    @Test
    fun `el host del vinculo va primero`() {
        val urls = AgentReachability.candidateBaseUrls(
            linkHost = "192.168.0.48", linkPort = 7341,
            savedAddresses = listOf("100.64.0.10", "192.168.0.48"),
        )

        assertEquals(listOf("https://192.168.0.48:7341", "https://100.64.0.10:7341"), urls)
    }

    @Test
    fun `sin addresses guardadas solo esta el host del vinculo`() {
        val urls = AgentReachability.candidateBaseUrls("192.168.0.48", 7341, emptyList())
        assertEquals(listOf("https://192.168.0.48:7341"), urls)
    }

    @Test
    fun `direcciones repetidas o en blanco no duplican candidatos`() {
        val urls = AgentReachability.candidateBaseUrls(
            linkHost = "192.168.0.48", linkPort = 7341,
            savedAddresses = listOf("100.64.0.10", "100.64.0.10", "", "  "),
        )

        assertEquals(listOf("https://192.168.0.48:7341", "https://100.64.0.10:7341"), urls)
    }

    @Test
    fun `todas las addresses de Tailscale se prueban con el puerto del vinculo`() {
        val urls = AgentReachability.candidateBaseUrls(
            linkHost = "192.168.0.48", linkPort = 7341,
            savedAddresses = listOf("100.64.0.10", "100.64.0.5"),
        )

        assertEquals(
            listOf("https://192.168.0.48:7341", "https://100.64.0.10:7341", "https://100.64.0.5:7341"),
            urls,
        )
    }

    @Test
    fun `nunca manda el token a una IP publica ni a algo que no sea IPv4`() {
        val urls = AgentReachability.candidateBaseUrls(
            linkHost = "192.168.0.48", linkPort = 7341,
            savedAddresses = listOf("8.8.8.8", "100.64.0.10", "2001:db8::1", "192.168.56.1", "evil.example.com"),
        )

        assertEquals(
            listOf("https://192.168.0.48:7341", "https://100.64.0.10:7341", "https://192.168.56.1:7341"),
            urls,
        )
    }

    @Test
    fun `Tailscale va antes que otras IP privadas del agente`() {
        val urls = AgentReachability.candidateBaseUrls(
            linkHost = "192.168.0.48", linkPort = 7341,
            savedAddresses = listOf("192.168.56.1", "10.0.0.5", "100.64.0.10"),
        )

        assertEquals(
            listOf("https://192.168.0.48:7341", "https://100.64.0.10:7341", "https://192.168.56.1:7341", "https://10.0.0.5:7341"),
            urls,
        )
    }

    @Test
    fun `rangos privados y de Tailscale exactos`() {
        val dentro = listOf("10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.0.48", "100.64.0.1", "100.127.255.255")
        val fuera = listOf("172.15.0.1", "172.32.0.1", "100.63.255.255", "100.128.0.1", "1.1.1.1", "300.1.1.1", "192.168.0", "")

        dentro.forEach { assertEquals(it, true, AgentReachability.isLanOrTailscale(it)) }
        fuera.forEach { assertEquals(it, false, AgentReachability.isLanOrTailscale(it)) }
    }
}
