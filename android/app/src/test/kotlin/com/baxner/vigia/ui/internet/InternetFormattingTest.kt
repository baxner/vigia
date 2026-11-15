package com.baxner.vigia.ui.internet

import com.baxner.vigia.data.agent.OutageBlame
import org.junit.Assert.assertEquals
import org.junit.Test

/** Formateo puro de duraciones y culpables (ver el encargo, "Caídas"). */
class InternetFormattingTest {

    @Test
    fun `una caida en curso se muestra como en curso sin importar la duracion`() {
        assertEquals("en curso", formatOutageDuration(durationSec = 0, ongoing = true))
        assertEquals("en curso", formatOutageDuration(durationSec = 12_345, ongoing = true))
    }

    @Test
    fun `menos de un minuto`() {
        assertEquals("menos de 1 min", formatOutageDuration(durationSec = 30, ongoing = false))
    }

    @Test
    fun `minutos sin horas`() {
        assertEquals("3 min", formatOutageDuration(durationSec = 3 * 60, ongoing = false))
    }

    @Test
    fun `horas y minutos`() {
        assertEquals("1 h 20 min", formatOutageDuration(durationSec = 60 * 60 + 20 * 60, ongoing = false))
    }

    @Test
    fun `horas exactas sin minutos`() {
        assertEquals("2 h", formatOutageDuration(durationSec = 2 * 3600, ongoing = false))
    }

    @Test
    fun `blameLabel traduce cada culpable del contrato`() {
        assertEquals("Tu router", blameLabel(OutageBlame.LAN))
        assertEquals("El módem", blameLabel(OutageBlame.MODEM))
        assertEquals("Tu proveedor", blameLabel(OutageBlame.ISP))
        assertEquals("No se sabe", blameLabel(OutageBlame.UNKNOWN))
    }
}
