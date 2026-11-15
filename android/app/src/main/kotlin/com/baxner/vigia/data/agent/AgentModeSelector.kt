package com.baxner.vigia.data.agent

/**
 * Decide si una de las redes que vigila el agente ([GET /api/v1/networks])
 * es la red actual del teléfono: "misma subred y router" (ver el contrato,
 * "Red del agente vs. red de la app" — el servidor va por cable y no conoce el
 * SSID, así que el SSID no entra en esta comparación). Pura — sin red, sin
 * Android — para poder probarla con JUnit plano.
 */
object AgentModeSelector {
    /**
     * [currentGateway]/[currentSubnet] son los de
     * [com.baxner.vigia.data.network.CurrentNetwork.gateway] y
     * `NetworkKey.subnetOf(network.cidr)`. `null` si falta alguno de los
     * dos (sin red usable no hay nada que comparar) o ninguna red del
     * agente coincide.
     *
     * Ignora las redes con `role == "outer"` (ver el contrato § "Seguridad"
     * → "Anillos"): esa es la red de delante del router (la de la ONT), y
     * "Red monitoreada" tiene que significar la red de casa. Si el teléfono
     * está conectado a la Wi-Fi de la ONT, la app escanea por su cuenta
     * (ver `ui/security/`, "Tu router visto desde aquí").
     */
    fun matchingNetwork(
        agentNetworks: List<AgentNetworkDto>,
        currentGateway: String?,
        currentSubnet: String?,
    ): AgentNetworkDto? {
        if (currentGateway.isNullOrBlank() || currentSubnet.isNullOrBlank()) return null
        return agentNetworks.firstOrNull {
            it.role != "outer" && it.gateway == currentGateway && it.subnet == currentSubnet
        }
    }

    /**
     * "Casa" para ver remoto por Tailscale (ver el encargo, fase 8: "todas
     * las pantallas fuera de casa"): la primera red que no sea `"outer"`
     * (mismo criterio que [matchingNetwork] — la de la ONT no cuenta como
     * casa). A diferencia de [matchingNetwork], no compara contra la red
     * actual del teléfono: viendo remoto, la red actual no tiene nada que
     * ver con cuál de las del agente mostrar. `null` solo si el agente no
     * vigila ninguna red interior (no debería pasar en la práctica: siempre
     * hay al menos la de casa).
     */
    fun primaryRemoteNetwork(agentNetworks: List<AgentNetworkDto>): AgentNetworkDto? =
        agentNetworks.firstOrNull { it.role != "outer" }
}
