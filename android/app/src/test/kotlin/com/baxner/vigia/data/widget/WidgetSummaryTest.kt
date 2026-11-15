package com.baxner.vigia.data.widget

import com.baxner.vigia.data.agent.InternetState
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * [WidgetSummaryDto] y sus funciones de actualización: pura, sin Android
 * (ver el encargo, "Lógica del resumen pura, con pruebas" — esta es esa
 * lógica). Cada `with*` solo debe tocar su propio campo y el reloj.
 */
class WidgetSummaryTest {
    private val t0: Instant = Instant.parse("2026-10-12T10:00:00Z")
    private val t1: Instant = Instant.parse("2026-10-12T10:05:00Z")

    @Test
    fun `withLinked true solo marca el vinculo y el reloj, deja lo demas igual`() {
        val previous = WidgetSummaryDto(linked = false, onlineDevices = 4, unreadAlerts = 2, internetState = "up", internetLatencyMs = 18, updatedAtEpochMs = t0.toEpochMilli())

        val updated = previous.withLinked(true, t1)

        assertTrue(updated.linked)
        assertEquals(4, updated.onlineDevices)
        assertEquals(2, updated.unreadAlerts)
        assertEquals("up", updated.internetState)
        assertEquals(18, updated.internetLatencyMs)
        assertEquals(t1.toEpochMilli(), updated.updatedAtEpochMs)
    }

    @Test
    fun `withLinked false borra todo lo demas (sin agente no hay nada que mostrar)`() {
        val previous = WidgetSummaryDto(linked = true, onlineDevices = 4, unreadAlerts = 2, internetState = "up", internetLatencyMs = 18, updatedAtEpochMs = t0.toEpochMilli())

        val updated = previous.withLinked(false, t1)

        assertFalse(updated.linked)
        assertEquals(0, updated.onlineDevices)
        assertEquals(0, updated.unreadAlerts)
        assertEquals("none", updated.internetState)
        assertNull(updated.internetLatencyMs)
        assertEquals(t1.toEpochMilli(), updated.updatedAtEpochMs)
    }

    @Test
    fun `withOnlineDevices solo cambia equipos y el reloj`() {
        val previous = WidgetSummaryDto(linked = true, unreadAlerts = 3, updatedAtEpochMs = t0.toEpochMilli())

        val updated = previous.withOnlineDevices(7, t1)

        assertEquals(7, updated.onlineDevices)
        assertEquals(3, updated.unreadAlerts)
        assertTrue(updated.linked)
        assertEquals(t1.toEpochMilli(), updated.updatedAtEpochMs)
    }

    @Test
    fun `withUnreadAlerts solo cambia alertas y el reloj`() {
        val previous = WidgetSummaryDto(linked = true, onlineDevices = 5, updatedAtEpochMs = t0.toEpochMilli())

        val updated = previous.withUnreadAlerts(9, t1)

        assertEquals(9, updated.unreadAlerts)
        assertEquals(5, updated.onlineDevices)
        assertEquals(t1.toEpochMilli(), updated.updatedAtEpochMs)
    }

    @Test
    fun `withInternet guarda estado y latencia, sin tocar equipos ni alertas`() {
        val previous = WidgetSummaryDto(linked = true, onlineDevices = 5, unreadAlerts = 1, updatedAtEpochMs = t0.toEpochMilli())

        val updated = previous.withInternet(InternetState.DEGRADED, 220, t1)

        assertEquals("degraded", updated.internetState)
        assertEquals(220, updated.internetLatencyMs)
        assertEquals(5, updated.onlineDevices)
        assertEquals(1, updated.unreadAlerts)
    }

    @Test
    fun `withInternet con DOWN tambien puede traer latencia nula`() {
        val updated = WidgetSummaryDto().withInternet(InternetState.DOWN, null, t1)

        assertEquals("down", updated.internetState)
        assertNull(updated.internetLatencyMs)
    }

    @Test
    fun `toWidgetWireState mapea los 4 casos, UNKNOWN cae en none`() {
        assertEquals("up", InternetState.UP.toWidgetWireState())
        assertEquals("degraded", InternetState.DEGRADED.toWidgetWireState())
        assertEquals("down", InternetState.DOWN.toWidgetWireState())
        assertEquals("none", InternetState.UNKNOWN.toWidgetWireState())
    }

    @Test
    fun `el resumen por defecto esta sin vincular y sin dato de internet`() {
        val default = WidgetSummaryDto()

        assertFalse(default.linked)
        assertEquals(0, default.onlineDevices)
        assertEquals(0, default.unreadAlerts)
        assertEquals("none", default.internetState)
        assertNull(default.internetLatencyMs)
        assertNull(default.updatedAtEpochMs)
    }
}
