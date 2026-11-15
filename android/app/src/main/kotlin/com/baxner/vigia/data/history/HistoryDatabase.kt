package com.baxner.vigia.data.history

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.TypeConverters

/**
 * Base de datos del historial local (ver `data/history/`).
 *
 * Versión 1: todavía no hay nada previo de qué migrar. Si el esquema cambia
 * a futuro, sumar un [androidx.room.migration.Migration] real en vez de
 * `fallbackToDestructiveMigration` — esto es el historial del usuario, no un
 * cache, y destruirlo de más no es gratis. El JSON de cada versión exportada
 * (ver `room.schemaLocation` en app/build.gradle.kts) queda en
 * app/schemas/ para poder escribir esas migraciones contra el esquema real.
 */
@Database(
    entities = [NetworkEntity::class, DeviceEntity::class, DeviceEventEntity::class],
    version = 1,
    exportSchema = true,
)
@TypeConverters(InstantConverter::class, DeviceEventTypeConverter::class)
abstract class HistoryDatabase : RoomDatabase() {
    abstract fun networkDao(): NetworkDao
    abstract fun deviceDao(): DeviceDao
    abstract fun eventDao(): DeviceEventDao

    companion object {
        private const val DATABASE_NAME = "vigia-history.db"

        fun create(context: Context): HistoryDatabase =
            Room.databaseBuilder(context, HistoryDatabase::class.java, DATABASE_NAME).build()
    }
}
