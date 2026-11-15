package com.baxner.vigia.data.history

import android.content.Context

/** De dónde sale el historial de la app. Un solo repositorio por proceso. */
object HistoryModule {
    @Volatile private var instance: HistoryRepository? = null

    fun repository(context: Context): HistoryRepository =
        instance ?: synchronized(this) {
            instance ?: create(context.applicationContext).also { instance = it }
        }

    private fun create(context: Context): HistoryRepository =
        RoomHistoryRepository(HistoryDatabase.create(context))
}
