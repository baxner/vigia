package com.baxner.vigia.data.radar

/*
 * Matemáticas y lógica puras de Bluetooth LE (ver AndroidBleScanner.kt y
 * AndroidBleSignalSource.kt), separadas del radio para poder probarlas con
 * JUnit plano.
 */

/**
 * Heurística de dirección aleatoria para teléfonos sin
 * `BluetoothDevice.getAddressType()` (API 35+, ver AndroidBleScanner.kt) o
 * cuando esa API devuelve `ADDRESS_TYPE_UNKNOWN`. Los dos bits más altos
 * del primer byte son, por la especificación de Bluetooth (Core Spec,
 * Vol 6, Part B, §1.3), el subtipo de una dirección ALEATORIA:
 * - `11` dirección estática.
 * - `01` privada resoluble (la que rota cada rato por privacidad; ver
 *   docs/plan.md: "muchos cambian de dirección cada ~15 min").
 * - `00`/`10` no alcanzan para afirmar nada: una dirección pública (de
 *   fábrica, asignada por IEEE) no sigue este convenio y puede caer en
 *   cualquier valor por pura casualidad del fabricante. Para esos dos
 *   casos se devuelve `false` (sin mentir: "no se puede saber" se trata
 *   como "no es aleatoria", no como un tercer estado).
 *
 * Nota honesta: incluso para `11`/`01` puede haber falsos positivos (una
 * dirección pública real cuyo primer byte cae por azar en ese patrón);
 * es una heurística, no una certeza, por eso [AndroidBleScanner] prefiere
 * la API real cuando existe.
 */
fun looksLikeRandomAddress(address: String): Boolean {
    val firstByte = address.substringBefore(':').trim().toIntOrNull(16) ?: return false
    val topBits = firstByte and 0xC0
    return topBits == 0xC0 || topBits == 0x40
}

/**
 * Agrega una lectura nueva a la lista de equipos BLE vistos en la sesión:
 * por dirección (si ya estaba, se reemplaza con la lectura nueva, no se
 * duplica) y ordenada del visto más reciente al más viejo.
 */
fun aggregateBleDevices(current: List<BleDevice>, reading: BleDevice): List<BleDevice> =
    (current.filterNot { it.address == reading.address } + reading).sortedByDescending { it.lastSeenMillis }

/** Estado de [BleScanner]: sin más matices que permiso y radio (a diferencia de Wi-Fi, no hay "no encontrado" para el listado general). */
fun bleScanState(hasPermission: Boolean, radioOn: Boolean): SignalSourceState = when {
    !hasPermission -> SignalSourceState.NO_PERMISSION
    !radioOn -> SignalSourceState.RADIO_OFF
    else -> SignalSourceState.OK
}
