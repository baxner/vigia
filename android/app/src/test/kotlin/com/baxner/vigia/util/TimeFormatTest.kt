package com.baxner.vigia.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.OffsetDateTime
import java.time.ZoneId
import java.util.Locale

class TimeFormatTest {

    @Test
    fun `la hora UTC del motor se muestra en la zona del telefono`() {
        // El motor Go en Android manda UTC: 04:14:49Z son las 10:14:49 p.m. en CDMX.
        val cdmx = ZoneId.of("America/Mexico_City")
        val mx = Locale.forLanguageTag("es-MX")
        val doce = formatClockTime("2026-10-06T04:14:49.123Z", is24Hour = false, zone = cdmx, locale = mx)
        assertTrue("salió: $doce", doce.startsWith("10:14:49"))
        assertEquals("22:14:49", formatClockTime("2026-10-06T04:14:49.123Z", is24Hour = true, zone = cdmx, locale = mx))
    }

    @Test
    fun `un timestamp con offset propio tambien se convierte`() {
        val text = formatClockTime("2026-10-05T22:14:49-06:00", is24Hour = true, zone = ZoneId.of("UTC"))
        assertEquals("04:14:49", text)
    }

    @Test
    fun `texto invalido o vacio no revienta`() {
        assertEquals("", formatClockTime("", is24Hour = true))
        assertEquals("ayer", formatClockTime("ayer", is24Hour = true))
    }
}

class RelativeTimeTest {
    private val now = OffsetDateTime.parse("2026-10-06T12:00:00Z")

    @Test
    fun `menos de 45 segundos es ahora mismo`() {
        assertEquals(RelativeTime.JustNow, relativeTimeFrom("2026-10-06T11:59:30Z", now))
    }

    @Test
    fun `entre 45 y 60 segundos cuenta en segundos`() {
        assertEquals(RelativeTime.Seconds(50), relativeTimeFrom("2026-10-06T11:59:10Z", now))
    }

    @Test
    fun `menos de una hora cuenta en minutos`() {
        assertEquals(RelativeTime.Minutes(2), relativeTimeFrom("2026-10-06T11:58:00Z", now))
    }

    @Test
    fun `una hora o mas cuenta en horas`() {
        assertEquals(RelativeTime.Hours(3), relativeTimeFrom("2026-10-06T09:00:00Z", now))
    }

    @Test
    fun `justo antes de 24 horas todavia cuenta en horas`() {
        assertEquals(RelativeTime.Hours(23), relativeTimeFrom("2026-10-05T13:00:00Z", now))
    }

    @Test
    fun `24 horas o mas cuenta en dias`() {
        assertEquals(RelativeTime.Days(1), relativeTimeFrom("2026-10-05T12:00:00Z", now))
        assertEquals(RelativeTime.Days(6), relativeTimeFrom("2026-09-30T00:00:00Z", now))
    }

    @Test
    fun `vacio o invalido no da un RelativeTime`() {
        assertNull(relativeTimeFrom("", now))
        assertNull(relativeTimeFrom("no es una fecha", now))
    }
}
