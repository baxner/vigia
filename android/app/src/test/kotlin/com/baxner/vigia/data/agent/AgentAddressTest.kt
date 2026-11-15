package com.baxner.vigia.data.agent

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.net.InetAddress

/** Lo que anuncia el servidor del agente por Bonjour (dns-sd -G): la IPv4 de la LAN debe ganar. */
class AgentAddressTest {
    private fun ip(s: String): InetAddress = InetAddress.getByName(s)

    @Test
    fun `la IPv4 de la LAN gana a las IPv6 que anuncia la Mac`() {
        val anunciadas = listOf(
            ip("2001:3::87d:4da9:20f1:5f00"),
            ip("fe80::1877:3096:a1b1:b82d"),
            ip("192.168.0.48"),
            ip("::1"),
        )
        assertEquals("192.168.0.48", pickAgentAddress(anunciadas)?.hostAddress)
    }

    @Test
    fun `sin IPv4, una IPv6 global antes que una de enlace`() {
        val anunciadas = listOf(ip("fe80::1"), ip("2001:db8::5"))
        assertEquals(ip("2001:db8::5"), pickAgentAddress(anunciadas))
    }

    @Test
    fun `nada util da null`() {
        assertNull(pickAgentAddress(emptyList()))
        assertNull(pickAgentAddress(listOf(ip("127.0.0.1"))))
    }

    @Test
    fun `agentBaseUrl pone corchetes a las IPv6`() {
        assertEquals("http://192.168.0.48:7341", agentBaseUrl("192.168.0.48", 7341))
        assertEquals("http://[2001:db8::5]:7341", agentBaseUrl("2001:db8::5", 7341))
        assertEquals("http://[fe80::1]:7341", agentBaseUrl("fe80::1%wlan0", 7341))
    }
}
