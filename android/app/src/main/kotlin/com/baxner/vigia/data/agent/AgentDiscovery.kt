package com.baxner.vigia.data.agent

import okhttp3.HttpUrl
import java.net.InetAddress
import java.net.Inet6Address
import java.net.Inet4Address
import android.content.Context
import android.net.nsd.NsdManager
import android.os.Build
import android.net.nsd.NsdServiceInfo
import android.net.wifi.WifiManager
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow

/** Un agente encontrado por mDNS (ver el contrato: servicio `_vigia._tcp`, TXT `v`/`id`/`name`/`cert`), listo para pasarle a la pantalla de vinculación. */
data class DiscoveredAgent(
    val id: String,
    val name: String,
    val host: String,
    val port: Int,
    val certSha256: String? = null,
)

/**
 * Los registros TXT de un [NsdServiceInfo] vienen como `Map<String, ByteArray?>`
 * (bytes crudos, UTF-8). Pura para poder probarla sin `NsdManager` real.
 */
fun decodeNsdTxtRecords(attributes: Map<String, ByteArray?>): Map<String, String> =
    attributes.mapNotNull { (key, value) -> value?.toString(Charsets.UTF_8)?.let { key to it } }.toMap()

/**
 * Descubrimiento del agente por mDNS/Bonjour (`_vigia._tcp`, ver el
 * contrato). Como el escaneo de red, necesita el `MulticastLock` prendido
 * mientras busca (si no, Android descarta los paquetes multicast para
 * ahorrar batería) — ver [com.baxner.vigia.data.network.WifiScanner], el
 * mismo patrón.
 */
class AgentDiscovery(context: Context) {
    private val appContext = context.applicationContext
    private val nsdManager = appContext.getSystemService(Context.NSD_SERVICE) as? NsdManager
    private val wifiManager = appContext.getSystemService(Context.WIFI_SERVICE) as? WifiManager
    private var multicastLock: WifiManager.MulticastLock? = null

    /**
     * Agentes vistos hasta ahora (se actualiza la lista completa cada vez
     * que aparece o se pierde uno). Sigue buscando mientras se colecte;
     * cancelar la colección detiene el descubrimiento y suelta el
     * MulticastLock (ver [awaitClose]).
     */
    fun discover(): Flow<List<DiscoveredAgent>> = callbackFlow {
        val manager = nsdManager
        if (manager == null) {
            trySend(emptyList())
            close()
            return@callbackFlow
        }

        val found = linkedMapOf<String, DiscoveredAgent>()
        acquireMulticastLock()

        val listener = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(serviceType: String) = Unit

            override fun onServiceFound(service: NsdServiceInfo) {
                resolve(manager, service) { resolved ->
                    if (resolved != null) {
                        found[service.serviceName] = resolved
                        trySend(found.values.toList())
                    }
                }
            }

            override fun onServiceLost(service: NsdServiceInfo) {
                found.remove(service.serviceName)
                trySend(found.values.toList())
            }

            override fun onDiscoveryStopped(serviceType: String) = Unit
            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                close()
            }
            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) = Unit
        }

        val started = runCatching { manager.discoverServices(ServiceType, NsdManager.PROTOCOL_DNS_SD, listener) }
        if (started.isFailure) {
            releaseMulticastLock()
            close()
        }

        awaitClose {
            runCatching { manager.stopServiceDiscovery(listener) }
            releaseMulticastLock()
        }
    }

    @Suppress("DEPRECATION") // resolveService: sin reemplazo simple compatible con minSdk 26 (registerServiceInfoCallback es API 34+).
    private fun resolve(manager: NsdManager, service: NsdServiceInfo, onResolved: (DiscoveredAgent?) -> Unit) {
        runCatching {
            manager.resolveService(
                service,
                object : NsdManager.ResolveListener {
                    override fun onResolveFailed(serviceInfo: NsdServiceInfo, errorCode: Int) = onResolved(null)

                    override fun onServiceResolved(serviceInfo: NsdServiceInfo) {
                        val txt = decodeNsdTxtRecords(serviceInfo.attributes)
                        // Un servidor anuncia varias direcciones (IPv4, IPv6 de enlace, a veces
                        // IPv6 que el teléfono no alcanza) y Android suele dar primero una
                        // IPv6: se elige la mejor de todas, con la IPv4 primero.
                        val candidates = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                            serviceInfo.hostAddresses
                        } else {
                            listOfNotNull(serviceInfo.host)
                        }
                        val host = pickAgentAddress(candidates)?.hostAddress
                        if (host == null) {
                            onResolved(null)
                            return
                        }
                        onResolved(
                            DiscoveredAgent(
                                id = txt["id"] ?: serviceInfo.serviceName,
                                name = txt["name"] ?: serviceInfo.serviceName,
                                host = host,
                                port = serviceInfo.port,
                                certSha256 = AgentTls.normalizeFingerprint(txt["cert"]),
                            ),
                        )
                    }
                },
            )
        }.onFailure { onResolved(null) }
    }

    private fun acquireMulticastLock() {
        if (multicastLock?.isHeld == true) return
        val lock = wifiManager?.createMulticastLock("vigia-agent-discovery") ?: return
        lock.setReferenceCounted(true)
        lock.acquire()
        multicastLock = lock
    }

    private fun releaseMulticastLock() {
        val lock = multicastLock ?: return
        if (lock.isHeld) lock.release()
        multicastLock = null
    }

    private companion object {
        const val ServiceType = "_vigia._tcp."
    }
}

/**
 * La dirección por la que conviene hablarle al agente: IPv4 privada primero
 * (la de la LAN), luego cualquier IPv4, luego una IPv6 global y al final una
 * de enlace local (esa necesita interfaz y casi nunca sirve en una URL).
 */
fun pickAgentAddress(addresses: List<InetAddress>): InetAddress? =
    addresses.filterNot { it.isLoopbackAddress }.minByOrNull { a ->
        when {
            a is Inet4Address && a.isSiteLocalAddress -> 0
            a is Inet4Address -> 1
            a is Inet6Address && !a.isLinkLocalAddress && !a.isSiteLocalAddress -> 2
            else -> 3
        }
    }

/** "http://host:puerto" válido también para IPv6 ("http://[2001:db8::1]:7341"). */
fun agentBaseUrl(host: String, port: Int): String =
    HttpUrl.Builder().scheme("http").host(host.substringBefore('%')).port(port).build().toString().trimEnd('/')
