package com.baxner.vigia.data.history

import androidx.room.TypeConverter
import java.time.Instant

/** Room no sabe guardar [Instant] directo: se persiste como epoch-millis. */
class InstantConverter {
    @TypeConverter
    fun toEpochMilli(instant: Instant?): Long? = instant?.toEpochMilli()

    @TypeConverter
    fun fromEpochMilli(value: Long?): Instant? = value?.let(Instant::ofEpochMilli)
}

/** Room no serializa enums por su cuenta: se persiste el nombre de la constante. */
class DeviceEventTypeConverter {
    @TypeConverter
    fun toName(type: DeviceEventType): String = type.name

    @TypeConverter
    fun fromName(value: String): DeviceEventType = DeviceEventType.valueOf(value)
}
