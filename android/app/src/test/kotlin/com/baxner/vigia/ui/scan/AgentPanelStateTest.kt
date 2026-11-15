package com.baxner.vigia.ui.scan

import com.baxner.vigia.data.agent.AgentPanelState
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Test

/** La hora del "último escaneo" de la tarjeta del agente sigue a los `event: scan` (ver [withScanEventAt]). */
class AgentPanelStateTest {

    private val t0: Instant = Instant.parse("2026-10-07T11:00:00Z")

    private val vigilando = AgentPanelState.Linked(
        agentName = "Mac-Estudio",
        reachable = true,
        monitoringCurrentNetwork = true,
        lastScanAt = t0,
    )

    @Test
    fun `un escaneo mas nuevo adelanta la hora`() {
        val despues = t0.plusSeconds(120)

        assertEquals(vigilando.copy(lastScanAt = despues), vigilando.withScanEventAt(despues))
    }

    @Test
    fun `uno mas viejo no la atrasa`() {
        assertSame(vigilando, vigilando.withScanEventAt(t0.minusSeconds(120)))
    }

    @Test
    fun `sin hora previa toma la del evento`() {
        val sinHora = vigilando.copy(lastScanAt = null)

        assertEquals(vigilando, sinHora.withScanEventAt(t0))
    }

    @Test
    fun `sin evento todavia no cambia`() {
        assertSame(vigilando, vigilando.withScanEventAt(null))
    }

    @Test
    fun `si no vigila esta red no se toca`() {
        val otraRed = vigilando.copy(monitoringCurrentNetwork = false, lastScanAt = null)

        assertSame(otraRed, otraRed.withScanEventAt(t0))
    }

    @Test
    fun `viendo remoto si actualiza la hora con el evento de escaneo`() {
        val remoto = vigilando.copy(monitoringCurrentNetwork = false, viewingRemote = true, lastScanAt = null)

        assertEquals(remoto.copy(lastScanAt = t0), remoto.withScanEventAt(t0))
    }

    @Test
    fun `sin vinculo no se toca`() {
        assertSame(AgentPanelState.NotLinked, AgentPanelState.NotLinked.withScanEventAt(t0))
    }
}
