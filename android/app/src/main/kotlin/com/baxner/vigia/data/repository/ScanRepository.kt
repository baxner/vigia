package com.baxner.vigia.data.repository

import com.baxner.vigia.data.model.ScanResult
import com.baxner.vigia.engine.mobile.Mobile
import com.baxner.vigia.engine.mobile.ProgressListener
import com.baxner.vigia.engine.mobile.Scanner
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.withContext
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.decodeFromString

/** El motor devolvió "escaneo cancelado": lo canceló la propia app, no es un error real. */
const val SCAN_CANCELLED_MESSAGE = "escaneo cancelado"

fun Throwable.isUserCancelled(): Boolean = message == SCAN_CANCELLED_MESSAGE

/**
 * Envuelve `vigia-core.aar`: llama al motor (bloqueante, por eso todo corre
 * en [Dispatchers.IO]) y decodifica el JSON que devuelve. El escaneo de red
 * tiene su propio [Scanner]; cada escaneo de puertos usa uno nuevo (ver
 * [scanPortsOnce]), porque el motor cancela la operación anterior de un
 * mismo Scanner (ver `Scanner.begin()` en mobile.go) y no queremos que
 * escanear puertos de un equipo corte el escaneo de red ni el de otro equipo.
 */
class ScanRepository {
    private val networkScanner: Scanner = Mobile.newScanner()

    /** Versión del motor Go embebido (`Mobile.version()`), para mostrar/depurar. */
    fun engineVersion(): String = Mobile.version()

    /** Fabricante por OUI de una MAC/BSSID, o "" si es aleatoria o desconocida. */
    fun vendorForMac(mac: String): String =
        runCatching { Mobile.vendorForMAC(mac) }.getOrDefault("")

    /**
     * Escanea [cidr] (obligatorio, "ip/prefijo") a través de [gateway].
     * [onProgress] se llama desde el hilo del motor (no el principal); quien
     * lo use y necesite tocar UI debe volcarlo a un StateFlow, no pintar
     * directo.
     */
    suspend fun scanNetwork(
        cidr: String,
        gateway: String,
        withPorts: Boolean,
        onProgress: (stage: String, done: Long, total: Long) -> Unit = { _, _, _ -> },
    ): Result<ScanResult> = withContext(Dispatchers.IO) {
        runCatching {
            val listener = ProgressListener { stage, done, total -> onProgress(stage, done, total) }
            val json = networkScanner.scan(cidr, gateway, withPorts, listener)
            VigiaJson.decodeFromString<ScanResult>(json)
        }
    }

    /** Corta el escaneo de red en curso (si hay uno). Se puede llamar desde cualquier hilo. */
    fun cancelNetworkScan() = networkScanner.cancel()

    companion object {
        /**
         * Puertos abiertos de un solo host, con un [Scanner] propio. [spec]
         * vacío = puertos por defecto del motor; o lista/rangos como
         * "22,80,8000-8100". Cancelar la corrutina corta el escaneo: el motor
         * no ve las interrupciones de Java, así que se le pide a él.
         */
        suspend fun scanPortsOnce(ip: String, spec: String = ""): Result<List<Int>> = coroutineScope {
            val scanner = Mobile.newScanner()
            val call = async(Dispatchers.IO) {
                runCatching { VigiaJson.decodeFromString(ListSerializer(Int.serializer()), scanner.scanPorts(ip, spec)) }
            }
            try {
                call.await()
            } catch (e: CancellationException) {
                scanner.cancel()
                throw e
            }
        }
    }
}
