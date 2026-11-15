package com.baxner.vigia.data.widget

import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** [SharedPrefsWidgetSummaryStore]: valor por defecto sin nada guardado, y round-trip de guardar/leer (mismo criterio que [com.baxner.vigia.data.history.RoomHistoryRepositoryTest]). */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class WidgetSummaryStoreTest {
    private val store = SharedPrefsWidgetSummaryStore(ApplicationProvider.getApplicationContext())

    @Test
    fun `sin nada guardado devuelve el resumen por defecto`() {
        assertEquals(WidgetSummaryDto(), store.get())
    }

    @Test
    fun `guarda y vuelve a leer el mismo resumen`() {
        val summary = WidgetSummaryDto(
            linked = true,
            onlineDevices = 6,
            unreadAlerts = 2,
            internetState = "up",
            internetLatencyMs = 18,
            updatedAtEpochMs = 1_760_000_000_000L,
        )

        store.set(summary)

        assertEquals(summary, store.get())
    }

    @Test
    fun `un segundo guardado reemplaza al primero por completo`() {
        store.set(WidgetSummaryDto(linked = true, onlineDevices = 6))
        store.set(WidgetSummaryDto(linked = true, onlineDevices = 1))

        assertEquals(1, store.get().onlineDevices)
    }
}
