package com.baxner.vigia.data.agent

import com.baxner.vigia.data.repository.VigiaJson
import kotlinx.serialization.decodeFromString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Las formas JSON de Internet del contrato (ver `docs/agente-api.md` §
 * "Internet (v1.4)"): el ejemplo de `InternetStatus` tiene que decodificar
 * entero (con [OutageDto]/[SpeedTestDto] anidados), la serie respeta
 * "`null` donde no hubo dato", y los defaults de un agente viejo no deben
 * reventar el parseo (mismo criterio que el resto de este cliente).
 */
class InternetDtoTest {

    private val contractInternetStatusJson = """
        {
          "state": "degraded",
          "since": "2026-10-11T14:58:00Z",
          "now": {"routerMs": 3.1, "modemMs": 4.0, "internetMs": 18.2, "dnsMs": 12.0, "lossPct": 2.5},
          "ipv6": "broken",
          "outages": [
            {"start": "2026-10-11T12:00:00Z", "end": "2026-10-11T12:03:20Z", "durationSec": 200, "blame": "isp"},
            {"start": "2026-10-11T15:05:00Z", "end": null, "durationSec": 0, "blame": "unknown"}
          ],
          "speedTests": [
            {"at": "2026-10-11T09:00:00Z", "downMbps": 95.4, "upMbps": 18.2, "latencyMs": 11.0, "loadedLatencyMs": 34.0, "error": null},
            {"at": "2026-10-10T09:00:00Z", "downMbps": null, "upMbps": null, "latencyMs": null, "loadedLatencyMs": null, "error": "timeout"}
          ]
        }
    """.trimIndent()

    @Test
    fun `InternetStatusDto decodifica el ejemplo del contrato`() {
        val status = VigiaJson.decodeFromString<InternetStatusDto>(contractInternetStatusJson)

        assertEquals(InternetState.DEGRADED, status.stateOrUnknown())
        assertEquals("2026-10-11T14:58:00Z", status.since)
        assertEquals(3.1, status.now.routerMs)
        assertEquals(4.0, status.now.modemMs)
        assertEquals(18.2, status.now.internetMs)
        assertEquals(12.0, status.now.dnsMs)
        assertEquals(2.5, status.now.lossPct, 0.0001)
        assertEquals(Ipv6State.BROKEN, status.ipv6OrUnknown())

        assertEquals(2, status.outages.size)
        val finished = status.outages[0]
        assertEquals(OutageBlame.ISP, finished.blameOrUnknown())
        assertEquals(200L, finished.durationSec)
        assertEquals("2026-10-11T12:03:20Z", finished.end)
        val ongoing = status.outages[1]
        assertNull(ongoing.end)
        assertEquals(OutageBlame.UNKNOWN, ongoing.blameOrUnknown())

        assertEquals(2, status.speedTests.size)
        val latest = status.speedTests[0]
        assertEquals(95.4, latest.downMbps)
        assertEquals(18.2, latest.upMbps)
        assertNull(latest.error)
        val failed = status.speedTests[1]
        assertNull(failed.downMbps)
        assertEquals("timeout", failed.error)
    }

    @Test
    fun `un estado o ipv6 que esta version no reconoce cae en UNKNOWN`() {
        val status = InternetStatusDto(state = "flaky", ipv6 = "weird")
        assertEquals(InternetState.UNKNOWN, status.stateOrUnknown())
        assertEquals(Ipv6State.UNKNOWN, status.ipv6OrUnknown())
    }

    @Test
    fun `InternetStatusDto por defecto no revienta sin agente v1_4`() {
        val status = VigiaJson.decodeFromString<InternetStatusDto>("{}")
        assertEquals(InternetState.UNKNOWN, status.stateOrUnknown())
        assertEquals(Ipv6State.UNKNOWN, status.ipv6OrUnknown())
        assertTrue(status.outages.isEmpty())
        assertTrue(status.speedTests.isEmpty())
    }

    @Test
    fun `serie respeta null donde no hubo dato`() {
        val json = """
            [
              {"at": "2026-10-11T14:00:00Z", "routerMs": 3.0, "modemMs": 4.0, "internetMs": 18.0, "dnsMs": 10.0, "lossPct": 0.0},
              {"at": "2026-10-11T14:01:00Z", "routerMs": null, "modemMs": null, "internetMs": null, "dnsMs": null, "lossPct": null}
            ]
        """.trimIndent()
        val points = VigiaJson.decodeFromString<List<InternetSeriesPointDto>>(json)
        assertEquals(2, points.size)
        assertEquals(18.0, points[0].internetMs)
        assertNull(points[1].internetMs)
        assertNull(points[1].lossPct)
    }

    @Test
    fun `InternetSettingsDto por defecto es 2 pruebas al dia`() {
        val settings = VigiaJson.decodeFromString<InternetSettingsDto>("{}")
        assertEquals(2, settings.speedTestsPerDay)
    }
}
