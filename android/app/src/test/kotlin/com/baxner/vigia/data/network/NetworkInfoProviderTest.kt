package com.baxner.vigia.data.network

import java.net.InetAddress
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * `InetAddress.getByName` con una IP literal (v4 o v6) no hace ninguna
 * consulta de red: el JDK la parsea directo. Por eso esto corre como test
 * unitario normal, sin Robolectric ni red real.
 */
class NetworkInfoProviderTest {

    @Test
    fun `formatCidr combina ip y prefijo con una barra`() {
        val address = InetAddress.getByName("192.168.0.105")
        assertEquals("192.168.0.105/24", formatCidr(address, 24))
    }

    @Test
    fun `formatCidr con prefijo 32 (host unico)`() {
        val address = InetAddress.getByName("10.0.0.1")
        assertEquals("10.0.0.1/32", formatCidr(address, 32))
    }

    @Test
    fun `firstIpv4AddressAndPrefix descarta IPv6 y elige la IPv4`() {
        val ipv6 = InetAddress.getByName("fe80::1") to 64
        val ipv4 = InetAddress.getByName("192.168.1.50") to 24

        val result = firstIpv4AddressAndPrefix(listOf(ipv6, ipv4))

        assertEquals("192.168.1.50", result?.first?.hostAddress)
        assertEquals(24, result?.second)
    }

    @Test
    fun `firstIpv4AddressAndPrefix respeta el orden cuando hay mas de una IPv4`() {
        val primera = InetAddress.getByName("192.168.1.50") to 24
        val segunda = InetAddress.getByName("192.168.1.51") to 24

        val result = firstIpv4AddressAndPrefix(listOf(primera, segunda))

        assertEquals("192.168.1.50", result?.first?.hostAddress)
    }

    @Test
    fun `firstIpv4AddressAndPrefix devuelve null si solo hay IPv6`() {
        val ipv6 = InetAddress.getByName("fe80::1") to 64
        assertNull(firstIpv4AddressAndPrefix(listOf(ipv6)))
    }

    @Test
    fun `firstIpv4AddressAndPrefix devuelve null con lista vacia`() {
        assertNull(firstIpv4AddressAndPrefix(emptyList()))
    }
}
