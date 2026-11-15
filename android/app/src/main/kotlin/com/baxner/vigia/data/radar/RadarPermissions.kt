package com.baxner.vigia.data.radar

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.content.ContextCompat

/**
 * Permisos de runtime que necesitan las fuentes de señal de "Buscar señal"
 * (fase 6, ver docs/plan.md y RadarContract.kt). Centralizado aquí porque,
 * a diferencia de ui/permissions/PermissionsState.kt (que los revisa una vez
 * al abrir la pantalla), estas fuentes los revisan en cada ciclo de su
 * propio bucle: si el usuario los revoca a mitad de una búsqueda, el
 * siguiente tick debe notarlo y pasar a [SignalSourceState.NO_PERMISSION],
 * no quedarse pidiendo datos que Android ya no va a dar.
 */
internal object RadarPermissions {
    /**
     * Wi-Fi: BSSID real y disparar `startScan()` piden ubicación precisa en
     * todas las versiones con escaneo Wi-Fi (26+); desde Android 13
     * (TIRAMISU) además pide "dispositivos cercanos" (ver el manifiesto).
     */
    fun hasWifiScan(context: Context): Boolean {
        val fineLocation = granted(context, Manifest.permission.ACCESS_FINE_LOCATION)
        val nearbyWifi = Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            granted(context, Manifest.permission.NEARBY_WIFI_DEVICES)
        return fineLocation && nearbyWifi
    }

    /**
     * Bluetooth LE: desde Android 12 (S) pide BLUETOOTH_SCAN; antes, el
     * escaneo BLE se autorizaba con ubicación (igual que Wi-Fi).
     */
    fun hasBleScan(context: Context): Boolean =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            granted(context, Manifest.permission.BLUETOOTH_SCAN)
        } else {
            granted(context, Manifest.permission.ACCESS_FINE_LOCATION)
        }

    private fun granted(context: Context, permission: String): Boolean =
        ContextCompat.checkSelfPermission(context, permission) == PackageManager.PERMISSION_GRANTED
}
