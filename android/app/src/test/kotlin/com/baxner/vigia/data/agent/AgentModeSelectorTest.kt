package com.baxner.vigia.data.agent

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Selección de modo (ver el encargo, punto 3): pura, sin red ni Android. La
 * app está en "Red monitoreada" cuando una red del agente coincide en
 * gateway Y subred con la actual — el SSID no entra (el servidor va por cable,
 * ver el contrato).
 */
class AgentModeSelectorTest {
    private val casa = AgentNetworkDto(key = "192.168.0.1|192.168.0.0/24", subnet = "192.168.0.0/24", gateway = "192.168.0.1")
    private val oficina = AgentNetworkDto(key = "10.0.0.1|10.0.0.0/24", subnet = "10.0.0.0/24", gateway = "10.0.0.1")

    @Test
    fun `coincide por gateway y subred`() {
        val match = AgentModeSelector.matchingNetwork(listOf(casa, oficina), currentGateway = "192.168.0.1", currentSubnet = "192.168.0.0/24")
        assertEquals(casa, match)
    }

    @Test
    fun `misma subred pero router distinto no coincide`() {
        val otraRed = AgentNetworkDto(key = "k", subnet = "192.168.0.0/24", gateway = "192.168.0.99")
        val match = AgentModeSelector.matchingNetwork(listOf(otraRed), currentGateway = "192.168.0.1", currentSubnet = "192.168.0.0/24")
        assertNull(match)
    }

    @Test
    fun `mismo gateway pero subred distinta no coincide`() {
        val otraRed = AgentNetworkDto(key = "k", subnet = "192.168.1.0/24", gateway = "192.168.0.1")
        val match = AgentModeSelector.matchingNetwork(listOf(otraRed), currentGateway = "192.168.0.1", currentSubnet = "192.168.0.0/24")
        assertNull(match)
    }

    @Test
    fun `sin gateway o sin subred actual nunca hay match`() {
        assertNull(AgentModeSelector.matchingNetwork(listOf(casa), currentGateway = null, currentSubnet = "192.168.0.0/24"))
        assertNull(AgentModeSelector.matchingNetwork(listOf(casa), currentGateway = "192.168.0.1", currentSubnet = null))
    }

    @Test
    fun `lista vacia de redes del agente nunca coincide`() {
        assertNull(AgentModeSelector.matchingNetwork(emptyList(), currentGateway = "192.168.0.1", currentSubnet = "192.168.0.0/24"))
    }

    @Test
    fun `una red con role outer nunca es Red monitoreada, aunque coincida gateway y subred`() {
        val ont = AgentNetworkDto(key = "192.168.100.1|192.168.100.0/24", subnet = "192.168.100.0/24", gateway = "192.168.100.1", role = "outer")
        val match = AgentModeSelector.matchingNetwork(listOf(ont), currentGateway = "192.168.100.1", currentSubnet = "192.168.100.0/24")
        assertNull(match)
    }

    @Test
    fun `con una red inner y otra outer coincidiendo, se queda con la inner`() {
        val casaInner = casa.copy(role = "inner")
        val ont = AgentNetworkDto(key = "o", subnet = casa.subnet, gateway = casa.gateway, role = "outer")
        val match = AgentModeSelector.matchingNetwork(listOf(ont, casaInner), currentGateway = casa.gateway, currentSubnet = casa.subnet)
        assertEquals(casaInner, match)
    }

    @Test
    fun `una red sin role (agente v1 1) sigue contando como inner`() {
        val agenteViejo = AgentNetworkDto(key = casa.key, subnet = casa.subnet, gateway = casa.gateway)
        val match = AgentModeSelector.matchingNetwork(listOf(agenteViejo), currentGateway = casa.gateway, currentSubnet = casa.subnet)
        assertEquals(agenteViejo, match)
    }

    // ---- primaryRemoteNetwork (ver el encargo, fase 8: "todas las pantallas fuera de casa") ----

    @Test
    fun `primaryRemoteNetwork se queda con la primera inner, sin comparar contra ninguna red actual`() {
        val match = AgentModeSelector.primaryRemoteNetwork(listOf(casa, oficina))
        assertEquals(casa, match)
    }

    @Test
    fun `primaryRemoteNetwork ignora las redes outer`() {
        val ont = AgentNetworkDto(key = "o", subnet = "192.168.100.0/24", gateway = "192.168.100.1", role = "outer")
        val match = AgentModeSelector.primaryRemoteNetwork(listOf(ont, casa))
        assertEquals(casa, match)
    }

    @Test
    fun `primaryRemoteNetwork null si el agente solo vigila redes outer`() {
        val ont = AgentNetworkDto(key = "o", subnet = "192.168.100.0/24", gateway = "192.168.100.1", role = "outer")
        assertNull(AgentModeSelector.primaryRemoteNetwork(listOf(ont)))
    }

    @Test
    fun `primaryRemoteNetwork null sin ninguna red`() {
        assertNull(AgentModeSelector.primaryRemoteNetwork(emptyList()))
    }

    @Test
    fun `primaryRemoteNetwork cuenta una red sin role como inner`() {
        val agenteViejo = AgentNetworkDto(key = casa.key, subnet = casa.subnet, gateway = casa.gateway)
        assertEquals(agenteViejo, AgentModeSelector.primaryRemoteNetwork(listOf(agenteViejo)))
    }
}
