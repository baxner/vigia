package com.baxner.vigia.data.widget

import android.content.Context
import com.baxner.vigia.data.repository.VigiaJson
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString

/**
 * Dónde vive el [WidgetSummaryDto] entre refrescos (ver `WidgetSummary.kt`):
 * `SharedPreferences`, igual criterio que [com.baxner.vigia.data.agent.AlertsStore]
 * — nada acá es secreto, solo contadores y un estado de texto. Separado de
 * la implementación real para poder probarla con JUnit plano.
 */
interface WidgetSummaryStore {
    fun get(): WidgetSummaryDto
    fun set(summary: WidgetSummaryDto)
}

class SharedPrefsWidgetSummaryStore(context: Context) : WidgetSummaryStore {
    private val prefs = context.applicationContext.getSharedPreferences(PrefsFileName, Context.MODE_PRIVATE)

    override fun get(): WidgetSummaryDto =
        prefs.getString(KeySummary, null)
            ?.let { runCatching { VigiaJson.decodeFromString<WidgetSummaryDto>(it) }.getOrNull() }
            ?: WidgetSummaryDto()

    override fun set(summary: WidgetSummaryDto) {
        prefs.edit().putString(KeySummary, VigiaJson.encodeToString(summary)).apply()
    }

    private companion object {
        const val PrefsFileName = "vigia_widget"
        const val KeySummary = "summary"
    }
}
