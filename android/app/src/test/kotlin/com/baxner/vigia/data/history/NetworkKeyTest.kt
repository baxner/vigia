package com.baxner.vigia.data.history

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class NetworkKeyTest {

    @Test
    fun `subnetOf saca la red del CIDR de la interfaz`() {
        assertEquals("192.168.0.0/24", NetworkKey.subnetOf("192.168.0.105/24"))
        assertEquals("10.0.0.0/8", NetworkKey.subnetOf("10.20.30.40/8"))
        assertEquals("172.16.4.0/22", NetworkKey.subnetOf("172.16.5.9/22"))
    }

    @Test
    fun `subnetOf no revienta con basura`() {
        assertNull(NetworkKey.subnetOf(null))
        assertNull(NetworkKey.subnetOf("192.168.0.105"))
        assertNull(NetworkKey.subnetOf("192.168.0/24"))
        assertNull(NetworkKey.subnetOf("192.168.0.1/40"))
    }

    @Test
    fun `sin SSID la clave lo marca con signo de interrogacion`() {
        assertEquals("?|192.168.0.4|192.168.0.0/24", NetworkKey.of(null, "192.168.0.4", "192.168.0.0/24").value)
    }
}
