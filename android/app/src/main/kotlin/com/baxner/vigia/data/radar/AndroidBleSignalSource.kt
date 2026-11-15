package com.baxner.vigia.data.radar

import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothManager
import android.bluetooth.le.BluetoothLeScanner
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanFilter
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/**
 * Señal de un equipo Bluetooth LE puntual (fase 6, "Buscar señal" sobre un
 * [SignalTarget.Ble]): un [SignalSample] por cada anuncio recibido, sin el
 * tope de ritmo de [AndroidBleScanner] (ahí sí hace falta para no inundar
 * la lista completa; aquí cada anuncio es justo lo que [RssiFilter.forBle]
 * espera filtrar). Filtra por dirección con [ScanFilter], así que si el
 * equipo usa una dirección aleatoria que rota (ver docs/plan.md: cada
 * ~15 min), el filtro deja de encontrarlo tras la rotación — esperado,
 * "sirve para buscarlo ahora, no para seguirlo días".
 */
class AndroidBleSignalSource(context: Context) : SignalSource {
    private val appContext = context.applicationContext
    private val bluetoothAdapter: BluetoothAdapter? =
        (appContext.getSystemService(Context.BLUETOOTH_SERVICE) as? BluetoothManager)?.adapter

    private val _state = MutableStateFlow(SignalSourceState.OK)
    override val state: StateFlow<SignalSourceState> = _state.asStateFlow()

    override fun samples(target: SignalTarget): Flow<SignalSample> {
        require(target is SignalTarget.Ble) { "AndroidBleSignalSource solo atiende SignalTarget.Ble, llegó $target" }
        return callbackFlow {
            var activeScanner: BluetoothLeScanner? = null

            val callback = object : ScanCallback() {
                override fun onScanResult(callbackType: Int, result: ScanResult) {
                    trySend(SignalSample(atMillis = System.currentTimeMillis(), rssi = result.rssi))
                }

                override fun onScanFailed(errorCode: Int) {
                    // Ver la misma nota en AndroidBleScanner: no hay un estado
                    // del contrato que describa bien un error de hardware o
                    // "demasiados escaneos"; solo se marca para reintentar.
                    activeScanner = null
                }
            }

            fun currentlyOk(): Boolean {
                val hasPermission = RadarPermissions.hasBleScan(appContext)
                val radioOn = bluetoothAdapter?.isEnabled == true
                _state.value = bleScanState(hasPermission, radioOn)
                return hasPermission && radioOn
            }

            fun startIfNeeded() {
                if (activeScanner != null) return
                if (!currentlyOk()) return
                val scanner = bluetoothAdapter?.bluetoothLeScanner ?: return
                val filter = ScanFilter.Builder().setDeviceAddress(target.address).build()
                val settings = ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build()
                val started = try {
                    scanner.startScan(listOf(filter), settings, callback)
                    true
                } catch (_: SecurityException) {
                    false
                } catch (_: IllegalStateException) {
                    false
                }
                if (started) activeScanner = scanner
            }

            fun stopIfRunning() {
                val scanner = activeScanner ?: return
                try {
                    scanner.stopScan(callback)
                } catch (_: SecurityException) {
                    // El permiso pudo revocarse con el escaneo corriendo.
                } catch (_: IllegalStateException) {
                    // El adaptador Bluetooth pudo apagarse mientras tanto.
                }
                activeScanner = null
            }

            startIfNeeded()

            val job = launch {
                while (isActive) {
                    if (!currentlyOk()) stopIfRunning() else startIfNeeded()
                    delay(StatePollMillis)
                }
            }

            awaitClose {
                job.cancel()
                stopIfRunning()
            }
        }
    }

    private companion object {
        // Solo para notar permiso/radio perdidos a mitad de la búsqueda; las
        // muestras en sí llegan por el callback del escaneo, a su propio ritmo.
        const val StatePollMillis = 1_000L
    }
}
