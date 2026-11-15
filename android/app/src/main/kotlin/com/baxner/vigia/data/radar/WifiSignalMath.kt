package com.baxner.vigia.data.radar

/*
 * Lógica pura de estados de AndroidWifiSignalSource.kt, separada del
 * WifiManager real para poder probarla con JUnit plano y fakes (ver el
 * encargo: "lógica de estados... con fakes").
 */

/** BSSID iguales sin distinguir mayúsculas (Android a veces da unas en mayúscula y otras no). */
fun bssidEquals(a: String?, b: String): Boolean = a != null && a.equals(b, ignoreCase = true)

/**
 * Estado para el AP *conectado* (se lee sin escanear, ver docs/plan.md):
 * sin permiso o con el Wi-Fi apagado, nada que mostrar; si el teléfono se
 * desconectó de ese BSSID exacto (roaming a otro AP, o se desconectó del
 * todo) se marca NOT_FOUND en vez de inventar una lectura.
 */
fun wifiConnectedState(hasPermission: Boolean, radioOn: Boolean, connectedToTarget: Boolean): SignalSourceState = when {
    !hasPermission -> SignalSourceState.NO_PERMISSION
    !radioOn -> SignalSourceState.RADIO_OFF
    !connectedToTarget -> SignalSourceState.NOT_FOUND
    else -> SignalSourceState.OK
}

/**
 * Estado para un AP que *no* es el conectado (se lee escaneando, ver
 * docs/plan.md): precedencia de arriba a abajo —
 * 1. Sin permiso o radio apagado: nada que hacer.
 * 2. Si apareció en el escaneo de este ciclo: OK (aunque el escaneo haya
 *    sido limitado — ver [throttled] —, los resultados en caché siguen
 *    siendo una lectura válida de que el AP está ahí).
 * 3. Si no apareció y el escaneo fue limitado por Android (resultados
 *    viejos, no se puede confiar en que de verdad ya no esté): SLOW_THROTTLED,
 *    la razón más específica y más útil ("no es que no esté, es que no
 *    puedo mirar seguido").
 * 4. Si no apareció en los últimos [missThreshold] escaneos (sin estar
 *    limitado): NOT_FOUND.
 * 5. Si no, todavía no hay motivo para alarmar: OK (va por el segundo o
 *    tercer intento, puede ser mala suerte de un escaneo puntual).
 */
fun wifiScanState(
    hasPermission: Boolean,
    radioOn: Boolean,
    foundThisScan: Boolean,
    throttled: Boolean,
    consecutiveMisses: Int,
    missThreshold: Int = 3,
): SignalSourceState = when {
    !hasPermission -> SignalSourceState.NO_PERMISSION
    !radioOn -> SignalSourceState.RADIO_OFF
    foundThisScan -> SignalSourceState.OK
    throttled -> SignalSourceState.SLOW_THROTTLED
    consecutiveMisses >= missThreshold -> SignalSourceState.NOT_FOUND
    else -> SignalSourceState.OK
}
