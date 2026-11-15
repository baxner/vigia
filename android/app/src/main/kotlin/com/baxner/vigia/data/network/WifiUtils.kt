package com.baxner.vigia.data.network

import android.net.wifi.ScanResult
import java.util.Locale
import kotlin.math.abs
import kotlin.math.log10
import kotlin.math.pow
import kotlin.math.roundToInt

/**
 * Funciones puras para interpretar un `android.net.wifi.ScanResult`: banda y
 * canal desde la frecuencia, seguridad desde `capabilities`, ancho de canal y
 * barras de señal desde el RSSI. Separadas de [WifiScanner] para poder
 * probarlas con JUnit plano, sin Android ni Robolectric.
 */

enum class WifiBand(val label: String) {
    GHZ_2_4("2.4 GHz"),
    GHZ_5("5 GHz"),
    GHZ_6("6 GHz"),
    UNKNOWN("?"),
}

data class WifiChannelInfo(val band: WifiBand, val channel: Int)

/**
 * Banda y canal Wi-Fi a partir de la frecuencia en MHz. Cubre 2.4/5/6 GHz
 * (incluido el canal 14 de Japón, fuera de la grilla de 5 MHz normal).
 */
fun channelInfoForFrequencyMhz(freqMhz: Int): WifiChannelInfo = when {
    freqMhz == 2484 -> WifiChannelInfo(WifiBand.GHZ_2_4, 14)
    freqMhz in 2412..2472 -> WifiChannelInfo(WifiBand.GHZ_2_4, (freqMhz - 2407) / 5)
    freqMhz in 5160..5885 -> WifiChannelInfo(WifiBand.GHZ_5, (freqMhz - 5000) / 5)
    freqMhz in 5955..7115 -> WifiChannelInfo(WifiBand.GHZ_6, (freqMhz - 5950) / 5)
    else -> WifiChannelInfo(WifiBand.UNKNOWN, -1)
}

enum class WifiSecurity(val label: String) {
    OPEN("Abierta"),
    WEP("WEP"),
    WPA("WPA"),
    WPA2("WPA2"),
    WPA3("WPA3"),
    UNKNOWN("Desconocida"),
}

/**
 * Seguridad a partir de `ScanResult.capabilities`, p. ej. "[WPA2-PSK-CCMP][ESS]".
 * Revisa de la más nueva a la más vieja porque en modo transición (p. ej.
 * WPA2/WPA3) aparecen varios tokens juntos y mostramos el más fuerte.
 */
fun securityFromCapabilities(capabilities: String): WifiSecurity {
    val caps = capabilities.uppercase()
    return when {
        caps.contains("WPA3") || caps.contains("SAE") || caps.contains("OWE") -> WifiSecurity.WPA3
        caps.contains("WPA2") || caps.contains("RSN") -> WifiSecurity.WPA2
        caps.contains("WPA") -> WifiSecurity.WPA
        caps.contains("WEP") -> WifiSecurity.WEP
        else -> WifiSecurity.OPEN
    }
}

/** Ancho de canal legible desde `ScanResult.channelWidth`. */
fun channelWidthLabel(channelWidth: Int): String = when (channelWidth) {
    ScanResult.CHANNEL_WIDTH_20MHZ -> "20 MHz"
    ScanResult.CHANNEL_WIDTH_40MHZ -> "40 MHz"
    ScanResult.CHANNEL_WIDTH_80MHZ -> "80 MHz"
    ScanResult.CHANNEL_WIDTH_160MHZ -> "160 MHz"
    ScanResult.CHANNEL_WIDTH_80MHZ_PLUS_MHZ -> "80+80 MHz"
    ScanResult.CHANNEL_WIDTH_320MHZ -> "320 MHz"
    else -> "? MHz"
}

/**
 * De RSSI (dBm) a 0–4 barras, con los mismos umbrales que usa Android
 * internamente (`WifiManager.calculateSignalLevel` con 5 niveles). Se
 * reimplementa a mano porque ese método de plataforma no se puede invocar
 * de forma determinística en tests unitarios JVM.
 */
fun rssiToBars(rssiDbm: Int): Int = when {
    rssiDbm >= -55 -> 4
    rssiDbm >= -66 -> 3
    rssiDbm >= -77 -> 2
    rssiDbm >= -88 -> 1
    else -> 0
}

/**
 * Fabricante de un punto de acceso. Los AP que publican varias redes (mesh,
 * red de invitados) usan BSSID "administrados localmente": la MAC base con el
 * bit 0x02 del primer byte encendido, que ya no está en la base OUI. Si la
 * BSSID tal cual no da fabricante, se prueba con ese bit apagado.
 */
fun vendorForBssid(bssid: String, lookup: (String) -> String): String {
    if (bssid.isBlank()) return ""
    lookup(bssid).takeIf { it.isNotBlank() }?.let { return it }
    if (bssid.indexOf(':') != 2) return ""
    val first = bssid.substring(0, 2).toIntOrNull(16) ?: return ""
    if (first and 0x02 == 0) return ""
    return lookup("%02x".format(first and 0x02.inv()) + bssid.substring(2))
}

/**
 * Generación Wi-Fi ("Wi-Fi 4/5/6/6E/7"), la nomenclatura de marketing de la
 * Wi-Fi Alliance para 802.11n/ac/ax/be.
 */
enum class WifiStandard(val label: String) {
    WIFI_4("Wi-Fi 4"),
    WIFI_5("Wi-Fi 5"),
    WIFI_6("Wi-Fi 6"),
    WIFI_6E("Wi-Fi 6E"),
    WIFI_7("Wi-Fi 7"),
}

/**
 * Generación Wi-Fi a partir de `ScanResult.wifiStandard` + la banda: 802.11ax
 * es "Wi-Fi 6E" en 6 GHz y "Wi-Fi 6" en el resto. `null` para legacy/
 * desconocido/11ad (WiGig, fuera de esta numeración) o cuando no hay dato.
 * `wifiStandard` solo existe desde API 30: quien llama debe pasar un valor
 * que no mapee a nada (p. ej. -1) en vez de leer ese campo en API viejas,
 * donde ni siquiera existe (ver [WifiScanner]).
 */
fun wifiStandardFor(wifiStandardRaw: Int, band: WifiBand): WifiStandard? = when (wifiStandardRaw) {
    ScanResult.WIFI_STANDARD_11N -> WifiStandard.WIFI_4
    ScanResult.WIFI_STANDARD_11AC -> WifiStandard.WIFI_5
    ScanResult.WIFI_STANDARD_11AX -> if (band == WifiBand.GHZ_6) WifiStandard.WIFI_6E else WifiStandard.WIFI_6
    ScanResult.WIFI_STANDARD_11BE -> WifiStandard.WIFI_7
    else -> null
}

/** Canal central y rango de frecuencias de un canal ancho (40/80/160/320 MHz). */
data class ChannelSpan(val centerChannel: Int, val startMhz: Int, val endMhz: Int)

/**
 * Centro y rango de un canal ancho, a partir de `centerFreq0`/`channelWidth`.
 * `null` en 20 MHz (ahí el canal principal ya es su propio centro, no hace
 * falta nada más) o sin un `centerFreq0` válido. El rango es el centro ± la
 * mitad del ancho: p. ej. centro 5250 MHz con 160 MHz de ancho da 5170-5330.
 * 80+80 MHz (no contiguo, con un segundo centro en `centerFreq1`) queda
 * fuera a propósito: un solo rango no lo representa bien.
 */
fun channelSpanFor(centerFreq0Mhz: Int, channelWidth: Int): ChannelSpan? {
    val widthMhz = when (channelWidth) {
        ScanResult.CHANNEL_WIDTH_40MHZ -> 40
        ScanResult.CHANNEL_WIDTH_80MHZ -> 80
        ScanResult.CHANNEL_WIDTH_160MHZ -> 160
        ScanResult.CHANNEL_WIDTH_320MHZ -> 320
        else -> return null
    }
    if (centerFreq0Mhz <= 0) return null
    val half = widthMhz / 2
    val centerChannel = channelInfoForFrequencyMhz(centerFreq0Mhz).channel
    return ChannelSpan(centerChannel = centerChannel, startMhz = centerFreq0Mhz - half, endMhz = centerFreq0Mhz + half)
}

/** true si `capabilities` incluye el token "WPS" (aprovisionamiento con PIN/botón). */
fun hasWps(capabilities: String): Boolean = capabilities.uppercase().contains("WPS")

/**
 * Distancia estimada en metros por pérdida en espacio libre (FSPL), despejando
 * la distancia de `RSSI(dBm) = -(27.55 - 20·log10(f MHz) + 20·log10(d m))`:
 * `d = 10^((27.55 - 20·log10(f MHz) + |RSSI|) / 20)`. A propósito aproximada
 * (asume espacio libre, sin paredes ni obstáculos): sirve para ordenar
 * "cerca/lejos", no como medición exacta.
 */
fun estimatedDistanceMeters(frequencyMhz: Int, rssiDbm: Int): Double {
    if (frequencyMhz <= 0) return 0.0
    val exponent = (27.55 - 20 * log10(frequencyMhz.toDouble()) + abs(rssiDbm)) / 20.0
    return 10.0.pow(exponent)
}

/**
 * Formatea una distancia en metros como la pide el encargo: "~0.1 m" con un
 * decimal por debajo de 1 metro, "~3 m"/"~14 m" redondeado a partir de ahí.
 * Redondea primero a décimas para que 0.96 m no quede como "~1.0 m" sino
 * como "~1 m" (ya cruzó el umbral tras el redondeo).
 */
fun formatDistanceMeters(distanceMeters: Double): String {
    val roundedTenths = (distanceMeters * 10).roundToInt() / 10.0
    val value = if (roundedTenths < 1.0) {
        String.format(Locale.US, "%.1f", roundedTenths)
    } else {
        roundedTenths.roundToInt().toString()
    }
    return "~$value m"
}

/** Atajo de [estimatedDistanceMeters] + [formatDistanceMeters] para construir un [WifiNetworkInfo]. */
fun estimatedDistanceLabel(frequencyMhz: Int, rssiDbm: Int): String =
    formatDistanceMeters(estimatedDistanceMeters(frequencyMhz, rssiDbm))

/** La IP sin el prefijo de un CIDR ("192.168.0.105/24" -> "192.168.0.105"). */
fun ipFromCidr(cidr: String?): String? = cidr?.substringBefore('/')?.takeIf { it.isNotBlank() }
