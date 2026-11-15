package com.baxner.vigia.data.widget

import android.content.Context
import androidx.glance.appwidget.updateAll
import com.baxner.vigia.data.agent.InternetState
import com.baxner.vigia.ui.widget.VigiaWidget
import java.time.Instant

/**
 * Puente entre quien ya tiene un dato fresco a mano (ver el encargo: "guarda
 * un resumen pequeño para el widget al refrescar") y el widget de pantalla
 * de inicio: guarda el [WidgetSummaryDto] actualizado en [WidgetSummaryStore]
 * y le pide a Glance que redibuje todas las instancias colocadas.
 *
 * Vive en `data/widget/` (no en `ui/widget/`) porque [com.baxner.vigia.ui.scan.ScanViewModel]
 * y [com.baxner.vigia.data.agent.AlertsRepository] son quienes llaman a estas
 * funciones, y ninguno de los dos debería depender de la pantalla del
 * widget en sí — solo [refreshGlanceWidget] toca [VigiaWidget], y a
 * propósito directo (sin una interfaz de registro que alguien tendría que
 * inicializar a tiempo): sin ella, un refresco que llega vía
 * [com.baxner.vigia.work.AlertsSyncWorker] en un proceso donde nunca se
 * creó ninguna `Activity` podría encontrar el "registro" vacío y perder el
 * redibujado. [runCatching] cubre lo demás: sin widgets colocados, o en
 * pruebas JVM sin el runtime de Glance, esto no debe tirar nada.
 */
object WidgetSummaryUpdater {
    suspend fun pushLinked(context: Context, linked: Boolean, store: WidgetSummaryStore = SharedPrefsWidgetSummaryStore(context)) {
        store.set(store.get().withLinked(linked, Instant.now()))
        refreshGlanceWidget(context)
    }

    suspend fun pushOnlineDevices(context: Context, onlineDevices: Int, store: WidgetSummaryStore = SharedPrefsWidgetSummaryStore(context)) {
        store.set(store.get().withOnlineDevices(onlineDevices, Instant.now()))
        refreshGlanceWidget(context)
    }

    suspend fun pushUnreadAlerts(context: Context, linked: Boolean, unreadAlerts: Int, store: WidgetSummaryStore = SharedPrefsWidgetSummaryStore(context)) {
        val current = store.get().withLinked(linked, Instant.now())
        store.set(if (linked) current.withUnreadAlerts(unreadAlerts, Instant.now()) else current)
        refreshGlanceWidget(context)
    }

    suspend fun pushInternet(context: Context, state: InternetState, latencyMs: Int?, store: WidgetSummaryStore = SharedPrefsWidgetSummaryStore(context)) {
        store.set(store.get().withInternet(state, latencyMs, Instant.now()))
        refreshGlanceWidget(context)
    }

    private suspend fun refreshGlanceWidget(context: Context) {
        runCatching { VigiaWidget().updateAll(context) }
    }
}
