package com.baxner.vigia.data.agent

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * La regla pura de "qué notificar" (ver el encargo, puntos 2 y 6: "sin
 * duplicados, orden, canal por tipo"). Sin MockWebServer ni Android: solo
 * [AlertNotificationPlanner.plan]/[AlertNotificationPlanner.nextCursor] con
 * listas armadas a mano.
 */
class AlertNotificationPlannerTest {

    // Reloj fijo: con el real, estas pruebas dejarían de pasar un día
    // después de la fecha de las alertas (ver MaxNotifyAge).
    private val now: Instant = Instant.parse("2026-10-07T08:00:00Z")

    private fun alert(id: Long, type: String = "NEW_DEVICE", at: String = "2026-10-07T07:10:00Z") = AlertDto(
        id = id, type = type, severity = "high", at = at,
        title = "t$id", body = "b$id",
    )

    @Test
    fun `nunca notifica uno ya notificado (id menor o igual al cursor)`() {
        val incoming = listOf(alert(1), alert(2), alert(3))

        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 2, settings = AlertSettingsDto(), now = now)

        assertEquals(listOf(3L), planned.map { it.id })
    }

    @Test
    fun `cursor en 0 (nunca se notifico nada) planea todo`() {
        val incoming = listOf(alert(1), alert(2))
        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 0, settings = AlertSettingsDto(), now = now)
        assertEquals(listOf(1L, 2L), planned.map { it.id })
    }

    @Test
    fun `ordena de la mas vieja a la mas nueva sin importar el orden de entrada`() {
        val incoming = listOf(alert(5), alert(3), alert(4))

        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 0, settings = AlertSettingsDto(), now = now)

        assertEquals(listOf(3L, 4L, 5L), planned.map { it.id })
    }

    @Test
    fun `colapsa ids repetidos (misma alerta vista dos veces)`() {
        val incoming = listOf(alert(1), alert(1), alert(2))

        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 0, settings = AlertSettingsDto(), now = now)

        assertEquals(listOf(1L, 2L), planned.map { it.id })
    }

    @Test
    fun `un tipo apagado en settings no se notifica`() {
        val incoming = listOf(alert(1, type = "NEW_DEVICE"), alert(2, type = "IMPORTANT_OFFLINE"))
        val settings = AlertSettingsDto(newDevice = false)

        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 0, settings = settings, now = now)

        assertEquals(listOf(2L), planned.map { it.id })
    }

    @Test
    fun `un tipo que esta version no reconoce se notifica igual (fail-open)`() {
        val incoming = listOf(alert(1, type = "SOMETHING_FROM_THE_FUTURE"))

        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 0, settings = AlertSettingsDto(), now = now)

        assertEquals(listOf(1L), planned.map { it.id })
    }

    @Test
    fun `los tipos del contrato (v1 1, Seguridad v1 2 e Internet v1 4) se pueden apagar y prender individualmente`() {
        val allOff = AlertSettingsDto(
            newDevice = false, importantOffline = false, importantOnline = false,
            agentBlind = false, agentRecovered = false,
            routerChanged = false, arpConflict = false, portMappingAdded = false, riskyService = false,
            internetDown = false, internetRestored = false,
        )
        val incoming = AlertType.entries.mapIndexed { index, type -> alert(index.toLong() + 1, type.name) }

        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 0, settings = allOff, now = now)

        assertTrue(planned.isEmpty())
    }

    @Test
    fun `una alerta de hace mas de un dia queda en la lista pero no se notifica`() {
        val incoming = listOf(
            alert(1, at = "2026-10-05T22:00:00Z"), // de anteayer: al vincular por primera vez
            alert(2, at = "2026-10-06T08:00:00Z"), // justo un día antes: todavía sí
            alert(3, at = "2026-10-07T07:59:00.123456789Z"), // con nanosegundos, como las fecha el agente en Go
        )

        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 0, settings = AlertSettingsDto(), now = now)

        assertEquals(listOf(2L, 3L), planned.map { it.id })
    }

    @Test
    fun `una fecha con zona horaria o ilegible no la deja fuera`() {
        val incoming = listOf(alert(1, at = "2026-10-07T01:30:00-06:00"), alert(2, at = "ayer"))

        val planned = AlertNotificationPlanner.plan(incoming, lastNotifiedId = 0, settings = AlertSettingsDto(), now = now)

        assertEquals(listOf(1L, 2L), planned.map { it.id })
    }

    @Test
    fun `lo viejo igual avanza el cursor para no volver a pedirlo`() {
        val seen = listOf(alert(7, at = "2026-09-01T00:00:00Z"))

        assertEquals(7L, AlertNotificationPlanner.nextCursor(seen, previous = 0))
    }

    // ---- nextCursor ----

    @Test
    fun `nextCursor avanza al id mas alto visto, no solo al notificado`() {
        // Una alerta de tipo apagado igual "se vio": no hay que volver a pedirla.
        val seen = listOf(alert(1, type = "NEW_DEVICE"), alert(2, type = "IMPORTANT_OFFLINE"))

        val next = AlertNotificationPlanner.nextCursor(seen, previous = 0)

        assertEquals(2L, next)
    }

    @Test
    fun `nextCursor sin nada visto se queda en el cursor anterior`() {
        assertEquals(5L, AlertNotificationPlanner.nextCursor(emptyList(), previous = 5))
    }
}
