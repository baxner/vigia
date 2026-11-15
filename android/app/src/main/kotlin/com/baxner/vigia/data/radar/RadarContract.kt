package com.baxner.vigia.data.radar

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow

/*
 * Fase 6, "Buscar señal" (ver docs/plan.md): el contrato entre las fuentes de
 * señal (radio, sensores) y las pantallas. Lo escribió el orquestador antes de
 * repartir el trabajo: las implementaciones reales viven en este paquete y la
 * UI en ui/radar/ y ui/bluetooth/, programadas contra estas interfaces.
 */

/** A quién se busca: un punto de acceso Wi-Fi o un equipo Bluetooth LE. */
sealed interface SignalTarget {
    /** Estable durante la búsqueda: el BSSID o la dirección Bluetooth. */
    val id: String

    /** Lo que ve el usuario ("Router-Lab", "Tira Govee"). */
    val label: String

    data class WifiAp(
        val bssid: String,
        val ssid: String,
        val frequencyMhz: Int,
        /** Responde a Wi-Fi RTT (802.11mc): hay distancia medida, no solo estimada. */
        val rttCapable: Boolean,
        /** Es el AP al que está conectado el teléfono: su señal se lee sin escanear. */
        val connected: Boolean,
        override val label: String,
    ) : SignalTarget {
        override val id: String get() = bssid
    }

    data class Ble(
        val address: String,
        val name: String?,
        val brand: String?,
        /** Potencia de transmisión anunciada (campo Tx Power Level), si el equipo la manda. */
        val txPowerDbm: Int?,
        override val label: String,
    ) : SignalTarget {
        override val id: String get() = address
    }
}

/**
 * Una lectura del objetivo. [rssi] en dBm (null si la lectura solo trae
 * distancia); [rttMeters] y [rttStdDevMeters] cuando hubo Wi-Fi RTT.
 */
data class SignalSample(
    val atMillis: Long,
    val rssi: Int?,
    val rttMeters: Double? = null,
    val rttStdDevMeters: Double? = null,
)

/**
 * Hacia dónde apunta el teléfono: grados desde el norte, en sentido horario
 * (0 ≤ azimuthDeg < 360). [reliable] es false si la brújula pide calibrarse.
 */
data class HeadingSample(val atMillis: Long, val azimuthDeg: Float, val reliable: Boolean)

/** Por qué las lecturas son lentas o no llegan: la UI lo explica en palabras. */
enum class SignalSourceState {
    OK,

    /** Android limita los escaneos de Wi-Fi (4 cada 2 min): lecturas lentas de otros AP. */
    SLOW_THROTTLED,
    NO_PERMISSION,

    /** Wi-Fi o Bluetooth apagado. */
    RADIO_OFF,

    /** El objetivo no se ha visto en los últimos escaneos. */
    NOT_FOUND,
}

/** Señal de un objetivo, mientras alguien la colecta. */
interface SignalSource {
    /** Lecturas de [target]; empezar a colectar arranca los escaneos que hagan falta y cancelar los detiene. */
    fun samples(target: SignalTarget): Flow<SignalSample>

    val state: StateFlow<SignalSourceState>
}

/** Rumbo del teléfono (brújula). */
interface HeadingSource {
    /** Vacío si el teléfono no tiene los sensores. */
    fun headings(): Flow<HeadingSample>
}

/** Un equipo Bluetooth LE visto en la sesión. */
data class BleDevice(
    val address: String,
    val name: String?,
    /** Marca por el identificador de fabricante del anuncio (lista oficial del Bluetooth SIG). */
    val brand: String?,
    val rssi: Int,
    val txPowerDbm: Int?,
    val lastSeenMillis: Long,
    /** Dirección aleatoria (privacidad): cambia cada tanto; sirve para buscarlo ahora, no para seguirlo días. */
    val randomAddress: Boolean,
)

/** Equipos Bluetooth LE cercanos, mientras alguien colecta (escaneo activo). */
interface BleScanner {
    /** Los vistos en la sesión, del más reciente al más viejo. */
    fun devices(): Flow<List<BleDevice>>

    val state: StateFlow<SignalSourceState>
}
