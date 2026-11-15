package com.baxner.vigia.data.network

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.wifi.WifiManager
import android.os.Build
import java.net.Inet4Address
import java.net.InetAddress

/**
 * La red local actual, tal como la ve la app: nada de esto lo puede dar el
 * motor Go en Android 11+ (el sistema no deja a las apps listar interfaces),
 * así que sale todo de [ConnectivityManager]/[android.net.LinkProperties].
 */
data class CurrentNetwork(
    /** "192.168.0.105/24", o null si no hay red local con IPv4 activa. */
    val cidr: String?,
    /** IP del router, o null si no se pudo determinar. */
    val gateway: String?,
    val isWifi: Boolean,
    /** Sin comillas; null si no es Wi-Fi o Android no nos deja leerlo. */
    val ssid: String?,
    val bssid: String?,
    /** dBm; null si no aplica. */
    val rssi: Int?,
    /** MHz; null si no aplica. Sirve para el resumen Wi-Fi del Panel (banda/canal). */
    val frequencyMhz: Int?,
    val linkSpeedMbps: Int?,
    val isEthernet: Boolean = false,
    /** Hay una VPN activa (p. ej. Tailscale). Los datos de arriba son de la LAN de debajo, no de la VPN. */
    val vpnActive: Boolean = false,
    /** La red de Android de esa LAN, para fijar a ella los sockets del escaneo. */
    val network: Network? = null,
) {
    /** Sin esto no hay nada que escanear: ni el motor ni la UI pueden seguir. */
    val hasUsableNetwork: Boolean get() = cidr != null
}

class NetworkInfoProvider(context: Context) {
    private val connectivityManager =
        context.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager
    private val wifiManager =
        context.getSystemService(Context.WIFI_SERVICE) as? WifiManager

    /**
     * La LAN a la que está conectado el teléfono (Wi-Fi o cable). No sirve
     * `activeNetwork`: con una VPN prendida (Tailscale) la red activa es la
     * VPN, con una IP 100.x/32 donde no hay nada que escanear. Tampoco se
     * escanea nunca la red del operador: sin Wi-Fi ni cable, `cidr` es null.
     */
    fun current(): CurrentNetwork {
        val cm = connectivityManager
        @Suppress("DEPRECATION") // allNetworks: la alternativa es un NetworkCallback, asíncrono.
        val all = cm?.allNetworks.orEmpty().mapNotNull { n -> cm?.getNetworkCapabilities(n)?.let { n to it } }
        val vpnActive = all.any { (_, caps) -> caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN) }
        val lan = all
            .filter { (_, caps) ->
                !caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN) &&
                    (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) ||
                        caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET))
            }
            .firstOrNull { (n, _) ->
                cm?.getLinkProperties(n)?.linkAddresses.orEmpty().any { it.address is Inet4Address }
            }
        val network = lan?.first
        val capabilities = lan?.second
        val linkProperties = network?.let { cm?.getLinkProperties(it) }
        val isWifi = capabilities?.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) == true
        val isEthernet = capabilities?.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) == true

        val addressPrefixPairs = linkProperties?.linkAddresses
            ?.map { la -> la.address to la.prefixLength }
            ?: emptyList()
        val cidr = firstIpv4AddressAndPrefix(addressPrefixPairs)
            ?.let { (address, prefixLength) -> formatCidr(address, prefixLength) }

        val gateway = linkProperties?.routes
            ?.firstOrNull { it.isDefaultRoute && it.gateway is Inet4Address }
            ?.gateway
            ?.hostAddress

        var ssid: String? = null
        var bssid: String? = null
        var rssi: Int? = null
        var frequencyMhz: Int? = null
        var linkSpeedMbps: Int? = null
        if (isWifi) {
            @Suppress("DEPRECATION")
            val info = wifiManager?.connectionInfo
            if (info != null) {
                val unknownSsid = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                    WifiManager.UNKNOWN_SSID
                } else {
                    "<unknown ssid>"
                }
                ssid = info.ssid
                    ?.trim('"')
                    ?.takeIf { it.isNotBlank() && it != unknownSsid }
                bssid = info.bssid?.takeIf { it.isNotBlank() && it != "02:00:00:00:00:00" }
                rssi = info.rssi
                frequencyMhz = info.frequency
                linkSpeedMbps = info.linkSpeed
            }
        }

        return CurrentNetwork(
            cidr = cidr,
            gateway = gateway,
            isWifi = isWifi,
            ssid = ssid,
            bssid = bssid,
            rssi = rssi,
            frequencyMhz = frequencyMhz,
            linkSpeedMbps = linkSpeedMbps,
            isEthernet = isEthernet,
            vpnActive = vpnActive,
            network = network,
        )
    }

    /**
     * Fija los sockets de toda la app (también los del motor Go) a [network],
     * o los suelta con null. Con una VPN que enruta la LAN, el escaneo iría
     * por el túnel; así sale directo por la Wi-Fi. Best-effort: si la VPN no
     * permite saltarla, Android devuelve false y todo sigue como estaba.
     */
    fun bindProcessTo(network: Network?): Boolean =
        runCatching { connectivityManager?.bindProcessToNetwork(network) == true }.getOrDefault(false)
}

/** "direccion/prefijo", exactamente lo que espera `Scanner.scan` (ver mobile.go). */
fun formatCidr(address: InetAddress, prefixLength: Int): String =
    "${address.hostAddress}/$prefixLength"

/**
 * De todas las direcciones de la interfaz (IPv4 e IPv6 mezcladas, como las da
 * `LinkProperties.linkAddresses`), la primera IPv4 con su prefijo. `Scanner.scan`
 * solo entiende IPv4, así que las IPv6 se descartan acá.
 */
fun firstIpv4AddressAndPrefix(addresses: List<Pair<InetAddress, Int>>): Pair<InetAddress, Int>? =
    addresses.firstOrNull { (address, _) -> address is Inet4Address }

