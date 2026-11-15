package com.baxner.vigia.data.agent

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * "Avisos al instante" (ver `docs/plan.md` § "Fase 5.1"): cuándo debe correr
 * [LiveWatchService] y qué debe decir su notificación — pura, sin Android,
 * sin red. Revisión de diseño del orquestador: el servicio ya NO se detiene
 * al salir de la red de casa (esa era la queja original, "llegan cuando
 * abro la app") — sigue vivo, en espera, mientras el interruptor esté
 * prendido y haya un agente vinculado.
 */
class LiveWatchPolicyTest {
    private val vigilando = AgentPanelState.Linked(
        agentName = "Mac-Estudio", reachable = true, monitoringCurrentNetwork = true, lastScanAt = null,
    )
    private val fueraDeCasa = AgentPanelState.Linked(
        agentName = "Mac-Estudio", reachable = true, monitoringCurrentNetwork = false, lastScanAt = null,
    )
    private val sinConexion = AgentPanelState.Linked(
        agentName = "Mac-Estudio", reachable = false, monitoringCurrentNetwork = false, lastScanAt = null,
    )

    // ---- shouldRun ----

    @Test
    fun `prendido y vigilando la red actual corre`() {
        assertTrue(LiveWatchPolicy.shouldRun(enabled = true, panelState = vigilando))
    }

    @Test
    fun `apagado no corre aunque este vigilando`() {
        assertFalse(LiveWatchPolicy.shouldRun(enabled = false, panelState = vigilando))
    }

    @Test
    fun `prendido y fuera de la red del agente SI corre (en espera, para reanudar solo al volver)`() {
        assertTrue(LiveWatchPolicy.shouldRun(enabled = true, panelState = fueraDeCasa))
    }

    @Test
    fun `prendido y sin respuesta del agente tambien corre (en espera)`() {
        assertTrue(LiveWatchPolicy.shouldRun(enabled = true, panelState = sinConexion))
    }

    @Test
    fun `prendido pero sin vinculo no corre`() {
        assertFalse(LiveWatchPolicy.shouldRun(enabled = true, panelState = AgentPanelState.NotLinked))
    }

    @Test
    fun `apagado y sin vinculo no corre`() {
        assertFalse(LiveWatchPolicy.shouldRun(enabled = false, panelState = AgentPanelState.NotLinked))
    }

    // ---- notificationKindFor ----

    @Test
    fun `vigilando la red de casa da WATCHING`() {
        assertEquals(LiveWatchNotificationKind.WATCHING, LiveWatchPolicy.notificationKindFor(vigilando))
    }

    @Test
    fun `vinculado pero fuera de la red del agente da AWAY_WAITING`() {
        assertEquals(LiveWatchNotificationKind.AWAY_WAITING, LiveWatchPolicy.notificationKindFor(fueraDeCasa))
    }

    @Test
    fun `vinculado pero sin respuesta del agente da AGENT_UNREACHABLE`() {
        assertEquals(LiveWatchNotificationKind.AGENT_UNREACHABLE, LiveWatchPolicy.notificationKindFor(sinConexion))
    }

    @Test
    fun `sin respuesta pesa mas que monitoringCurrentNetwork (no deberia pasar junto, pero por si acaso)`() {
        val inconsistente = AgentPanelState.Linked(
            agentName = "x", reachable = false, monitoringCurrentNetwork = true, lastScanAt = null,
        )
        assertEquals(LiveWatchNotificationKind.AGENT_UNREACHABLE, LiveWatchPolicy.notificationKindFor(inconsistente))
    }

    @Test
    fun `sin vinculo cae en AWAY_WAITING por default (no deberia llamarse, pero no debe lanzar)`() {
        assertEquals(LiveWatchNotificationKind.AWAY_WAITING, LiveWatchPolicy.notificationKindFor(AgentPanelState.NotLinked))
    }
}
