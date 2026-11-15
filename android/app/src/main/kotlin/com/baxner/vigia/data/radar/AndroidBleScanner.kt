package com.baxner.vigia.data.radar

import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothManager
import android.bluetooth.le.BluetoothLeScanner
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.os.Build
import java.util.concurrent.atomic.AtomicReference
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
 * Equipos Bluetooth LE cercanos (fase 6, ver docs/plan.md "Bluetooth" y
 * RadarContract.kt). Escanea en [ScanSettings.SCAN_MODE_LOW_LATENCY] solo
 * mientras alguien colecta [devices]; cancelar la colección detiene el
 * escaneo. El estado se revisa en cada ciclo (permiso o radio pueden
 * cambiar a mitad de una búsqueda, ver RadarPermissions.kt).
 */
class AndroidBleScanner(
    context: Context,
    private val companies: BluetoothCompanies,
) : BleScanner {
    // Solo el de la aplicación: es un singleton de proceso, y guardar el de la
    // Activity que lo pidió primero la retendría en memoria para siempre.
    private val appContext = context.applicationContext
    private val bluetoothAdapter: BluetoothAdapter? =
        (appContext.getSystemService(Context.BLUETOOTH_SERVICE) as? BluetoothManager)?.adapter

    private val _state = MutableStateFlow(SignalSourceState.OK)
    override val state: StateFlow<SignalSourceState> = _state.asStateFlow()

    override fun devices(): Flow<List<BleDevice>> = callbackFlow {
        val devicesRef = AtomicReference<List<BleDevice>>(emptyList())
        var activeScanner: BluetoothLeScanner? = null

        val callback = object : ScanCallback() {
            override fun onScanResult(callbackType: Int, result: ScanResult) {
                toBleDevice(result)?.let { reading ->
                    devicesRef.getAndUpdate { current -> aggregateBleDevices(current, reading) }
                }
            }

            override fun onBatchScanResults(results: MutableList<ScanResult>) {
                for (result in results) onScanResult(ScanSettings.CALLBACK_TYPE_ALL_MATCHES, result)
            }

            override fun onScanFailed(errorCode: Int) {
                // Ninguno de los SCAN_FAILED_* de Android mapea bien a los
                // estados del contrato (no es NO_PERMISSION ni RADIO_OFF
                // necesariamente: puede ser "demasiados escaneos" o un error
                // interno). En vez de forzar un estado que podría mentir, solo
                // se marca para reintentar: el próximo ciclo del bucle de abajo
                // vuelve a intentar iniciar el escaneo.
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
            val settings = ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build()
            val started = try {
                scanner.startScan(null, settings, callback)
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
                if (!currentlyOk()) {
                    stopIfRunning()
                } else {
                    startIfNeeded()
                }
                // Tope de ~4 actualizaciones por segundo que pide el encargo:
                // los anuncios BLE llegan mucho más seguido (decenas por
                // segundo entre todos los equipos cercanos), así que se agregan
                // en [devicesRef] todo el tiempo pero solo se manda la lista
                // a este ritmo.
                trySend(devicesRef.get())
                delay(TickMillis)
            }
        }

        awaitClose {
            job.cancel()
            stopIfRunning()
        }
    }

    private fun toBleDevice(result: ScanResult): BleDevice? {
        val device = result.device ?: return null
        val address = device.address ?: return null
        val record = result.scanRecord

        // Nombre del anuncio (ScanRecord), NO BluetoothDevice.getName(): ese
        // pide BLUETOOTH_CONNECT desde Android 12 (ver el encargo).
        val name = record?.deviceName?.takeIf { it.isNotBlank() }

        // "Primera clave" de manufacturerSpecificData: SparseArray la mantiene
        // ordenada por clave, así que keyAt(0) es la más chica, no
        // necesariamente la primera tal cual venía en los bytes del anuncio.
        val manufacturerData = record?.manufacturerSpecificData
        val companyId = manufacturerData?.takeIf { it.size() > 0 }?.keyAt(0)
        val brand = companyId?.let(companies::brandFor)

        val txPower = record?.txPowerLevel?.takeIf { it != Int.MIN_VALUE }

        return BleDevice(
            address = address,
            name = name,
            brand = brand,
            rssi = result.rssi,
            txPowerDbm = txPower,
            lastSeenMillis = System.currentTimeMillis(),
            randomAddress = isRandomAddress(device, address),
        )
    }

    /**
     * `BluetoothDevice.getAddressType()` es API 35+ (ver el reporte: se
     * comprobó contra el SDK local que la constante existe desde API 34
     * pero el método público recién en 35/VANILLA_ICE_CREAM). Antes de esa
     * versión, o si la devuelve como `ADDRESS_TYPE_UNKNOWN`, se cae a la
     * heurística de [looksLikeRandomAddress].
     */
    private fun isRandomAddress(device: BluetoothDevice, address: String): Boolean {
        if (Build.VERSION.SDK_INT >= 35) {
            when (runCatching { device.addressType }.getOrDefault(BluetoothDevice.ADDRESS_TYPE_UNKNOWN)) {
                BluetoothDevice.ADDRESS_TYPE_RANDOM -> return true
                BluetoothDevice.ADDRESS_TYPE_PUBLIC -> return false
                else -> Unit // ANONYMOUS/UNKNOWN: cae a la heurística.
            }
        }
        return looksLikeRandomAddress(address)
    }

    private companion object {
        const val TickMillis = 250L // ~4 actualizaciones por segundo.
    }
}
