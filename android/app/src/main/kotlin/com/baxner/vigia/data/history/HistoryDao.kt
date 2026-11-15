package com.baxner.vigia.data.history

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Update
import java.time.Instant
import kotlinx.coroutines.flow.Flow

@Dao
interface NetworkDao {
    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertIfAbsent(network: NetworkEntity)

    @Query("UPDATE networks SET lastSeen = :at WHERE `key` = :key")
    suspend fun touchLastSeen(key: String, at: Instant)

    /** Registra [key] si es la primera vez, y en cualquier caso actualiza su `lastSeen`. */
    suspend fun touch(key: String, at: Instant) {
        insertIfAbsent(NetworkEntity(key = key, firstSeen = at, lastSeen = at))
        touchLastSeen(key, at)
    }
}

@Dao
interface DeviceDao {
    @Insert
    suspend fun insert(entity: DeviceEntity): Long

    @Update
    suspend fun update(entity: DeviceEntity)

    /** Snapshot de los conocidos de [networkKey] (para reconciliar un escaneo). */
    @Query("SELECT * FROM devices WHERE networkKey = :networkKey")
    suspend fun listByNetwork(networkKey: String): List<DeviceEntity>

    // Sin ORDER BY: lastIp es texto ("192.168.0.105" < "192.168.0.29" como
    // string) y SQLite no tiene una forma limpia de partir el punto para
    // ordenar numérico. El orden por IP real lo hace RoomHistoryRepository
    // en Kotlin, después de mapear a KnownDevice.
    @Query("SELECT * FROM devices WHERE networkKey = :networkKey")
    fun observeByNetwork(networkKey: String): Flow<List<DeviceEntity>>

    @Query("SELECT * FROM devices WHERE id = :id")
    fun observeById(id: Long): Flow<DeviceEntity?>

    @Query(
        "UPDATE devices SET custom_name = :name, custom_typeId = :typeId, custom_notes = :notes, " +
            "custom_location = :location, custom_important = :important WHERE id = :id",
    )
    suspend fun updateCustomization(
        id: Long,
        name: String?,
        typeId: String?,
        notes: String?,
        location: String?,
        important: Boolean,
    )

    @Query("DELETE FROM devices WHERE id = :id")
    suspend fun delete(id: Long)
}

@Dao
interface DeviceEventDao {
    @Insert
    suspend fun insertAll(events: List<DeviceEventEntity>)

    // Empate en `at` DESC con `id` DESC: WENT_OFFLINE se fecha con el lastSeen real (ver
    // DeviceHistoryEngine.missing), que puede coincidir con el instante de otro evento ya
    // guardado (p. ej. un FIRST_SEEN de un equipo que solo se vio una vez); el id más alto
    // es el que se grabó después, así que desempata hacia "más reciente primero".
    @Query("SELECT * FROM device_events WHERE deviceId = :deviceId ORDER BY at DESC, id DESC")
    fun observeForDevice(deviceId: Long): Flow<List<DeviceEventEntity>>

    @Query("DELETE FROM device_events WHERE deviceId = :deviceId")
    suspend fun deleteForDevice(deviceId: Long)
}
