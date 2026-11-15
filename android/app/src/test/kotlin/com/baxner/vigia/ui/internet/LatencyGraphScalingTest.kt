package com.baxner.vigia.ui.internet

import com.baxner.vigia.data.agent.OutageDto
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Escalado puro de la gráfica de latencia (ver el encargo: "huecos, mínimo y
 * máximo"). Sin Canvas ni Compose — ver `LatencyChart` en
 * `ui/internet/InternetScreen.kt`, que solo traduce esto a píxeles.
 */
class LatencyGraphScalingTest {

    @Test
    fun `rangeOf ignora los huecos para el minimo y el maximo`() {
        val range = LatencyGraphScaling.rangeOf(listOf(10.0, null, 30.0, null, 20.0))
        assertEquals(10.0, range.min, 0.0001)
        assertEquals(30.0, range.max, 0.0001)
    }

    @Test
    fun `rangeOf sin ningun dato da un rango que no revienta`() {
        val range = LatencyGraphScaling.rangeOf(listOf(null, null, null))
        assertEquals(0.0, range.min, 0.0001)
        assertEquals(1.0, range.max, 0.0001)
        // normalize no debe dividir por cero ni devolver NaN/Infinity.
        assertEquals(0f, range.normalize(0.0))
    }

    @Test
    fun `rangeOf con un solo valor da un rango de 1 ms alrededor`() {
        val range = LatencyGraphScaling.rangeOf(listOf(15.0))
        assertEquals(14.0, range.min, 0.0001)
        assertEquals(16.0, range.max, 0.0001)
    }

    @Test
    fun `normalize lleva el minimo a 0 y el maximo a 1`() {
        val range = LatencyGraphScaling.rangeOf(listOf(10.0, 30.0))
        assertEquals(0f, range.normalize(10.0), 0.0001f)
        assertEquals(1f, range.normalize(30.0), 0.0001f)
        assertEquals(0.5f, range.normalize(20.0), 0.0001f)
    }

    @Test
    fun `segments agrupa tramos contiguos y corta en cada hueco`() {
        // [A, A, hueco, B, hueco, hueco, C, C, C]
        val values = listOf(1.0, 2.0, null, 3.0, null, null, 4.0, 5.0, 6.0)
        val segments = LatencyGraphScaling.segments(values)
        assertEquals(listOf(listOf(0, 1), listOf(3), listOf(6, 7, 8)), segments)
    }

    @Test
    fun `segments sin ningun hueco da un solo tramo`() {
        val segments = LatencyGraphScaling.segments(listOf(1.0, 2.0, 3.0))
        assertEquals(listOf(listOf(0, 1, 2)), segments)
    }

    @Test
    fun `segments con todo hueco no da ningun tramo`() {
        assertTrue(LatencyGraphScaling.segments(listOf(null, null)).isEmpty())
    }

    @Test
    fun `outageMarkerIndices marca los puntos dentro de una caida terminada`() {
        val timestamps = listOf(
            "2026-10-11T11:58:00Z",
            "2026-10-11T12:00:00Z",
            "2026-10-11T12:02:00Z",
            "2026-10-11T12:05:00Z",
        )
        val outages = listOf(OutageDto(start = "2026-10-11T12:00:00Z", end = "2026-10-11T12:03:00Z", durationSec = 180, blame = "isp"))
        val marked = LatencyGraphScaling.outageMarkerIndices(timestamps, outages)
        assertEquals(setOf(1, 2), marked)
    }

    @Test
    fun `outageMarkerIndices extiende una caida en curso hasta ahora`() {
        val timestamps = listOf("2026-10-11T12:00:00Z", "2026-10-11T12:30:00Z", "2026-10-11T13:00:00Z")
        val outages = listOf(OutageDto(start = "2026-10-11T12:00:00Z", end = null, durationSec = 0, blame = "unknown"))
        val now = Instant.parse("2026-10-11T12:45:00Z")
        val marked = LatencyGraphScaling.outageMarkerIndices(timestamps, outages, now)
        assertEquals(setOf(0, 1), marked)
    }

    @Test
    fun `outageMarkerIndices sin caidas no marca nada`() {
        val marked = LatencyGraphScaling.outageMarkerIndices(listOf("2026-10-11T12:00:00Z"), emptyList())
        assertTrue(marked.isEmpty())
    }
}
