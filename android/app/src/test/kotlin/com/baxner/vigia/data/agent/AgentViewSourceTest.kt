package com.baxner.vigia.data.agent

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Qué fuente debe mostrar la app (ver el encargo, fase 8: "todas las
 * pantallas fuera de casa por Tailscale") según la red actual + el vínculo +
 * si el agente contestó remoto — pura, sin red ni Android (ver
 * [AgentModeController.evaluate], que ya calculó cada parámetro antes de
 * preguntarle esto a [AgentViewSourceSelector.decide]).
 */
class AgentViewSourceTest {

    @Test
    fun `la red de casa manda siempre, aunque remoto tambien conteste`() {
        val source = AgentViewSourceSelector.decide(localMatch = true, remoteReachable = true, remoteViewPreferred = true)
        assertEquals(AgentViewSource.Monitored, source)
    }

    @Test
    fun `red de casa manda incluso si el usuario prefiere remoto`() {
        val source = AgentViewSourceSelector.decide(localMatch = true, remoteReachable = false, remoteViewPreferred = true)
        assertEquals(AgentViewSource.Monitored, source)
    }

    @Test
    fun `fuera de casa, remoto alcanzable y preferido, ve la casa remoto`() {
        val source = AgentViewSourceSelector.decide(localMatch = false, remoteReachable = true, remoteViewPreferred = true)
        assertEquals(AgentViewSource.Remote, source)
    }

    @Test
    fun `fuera de casa, remoto alcanzable pero el usuario prefiere la red actual`() {
        val source = AgentViewSourceSelector.decide(localMatch = false, remoteReachable = true, remoteViewPreferred = false)
        assertEquals(AgentViewSource.CurrentNetwork, source)
    }

    @Test
    fun `fuera de casa y remoto inalcanzable, red actual aunque se prefiera remoto`() {
        val source = AgentViewSourceSelector.decide(localMatch = false, remoteReachable = false, remoteViewPreferred = true)
        assertEquals(AgentViewSource.CurrentNetwork, source)
    }

    @Test
    fun `sin nada que ofrecer, red actual`() {
        val source = AgentViewSourceSelector.decide(localMatch = false, remoteReachable = false, remoteViewPreferred = false)
        assertEquals(AgentViewSource.CurrentNetwork, source)
    }
}
