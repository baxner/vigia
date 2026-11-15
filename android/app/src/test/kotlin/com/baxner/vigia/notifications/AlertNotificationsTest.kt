package com.baxner.vigia.notifications

import com.baxner.vigia.data.agent.AlertType
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Canal de Android por tipo de alerta (ver el encargo, punto 6: "canal por
 * tipo", y docs/agente-api.md § "Alertas v1.1" → "Notificaciones en la
 * app"). [AlertType.channelId] es pura (un `when` sin Android real de por
 * medio), así que no hace falta Robolectric para probarla.
 */
class AlertNotificationsTest {
    @Test
    fun `NEW_DEVICE va al canal de equipos nuevos`() {
        assertEquals(AlertChannels.NEW_DEVICES, AlertType.NEW_DEVICE.channelId())
    }

    @Test
    fun `IMPORTANT_OFFLINE e IMPORTANT_ONLINE van al canal de equipos importantes`() {
        assertEquals(AlertChannels.IMPORTANT_DEVICES, AlertType.IMPORTANT_OFFLINE.channelId())
        assertEquals(AlertChannels.IMPORTANT_DEVICES, AlertType.IMPORTANT_ONLINE.channelId())
    }

    @Test
    fun `AGENT_BLIND y AGENT_RECOVERED van al canal de estado del agente`() {
        assertEquals(AlertChannels.AGENT_STATUS, AlertType.AGENT_BLIND.channelId())
        assertEquals(AlertChannels.AGENT_STATUS, AlertType.AGENT_RECOVERED.channelId())
    }

    @Test
    fun `un tipo sin reconocer (null) cae en el canal de menor prioridad`() {
        val unknownType: AlertType? = null
        assertEquals(AlertChannels.AGENT_STATUS, unknownType.channelId())
    }
}
