package com.baxner.vigia.data.model

import kotlinx.serialization.Serializable

/**
 * Espejo en Kotlin de `core.ScanResult` (Go). Todos los campos tienen un
 * valor por defecto porque el motor usa `omitempty`: muchas claves pueden
 * faltar directamente en el JSON. El [com.baxner.vigia.data.repository.VigiaJson]
 * además activa `coerceInputValues`, por si algún campo sin `omitempty`
 * (como `sources`) llega como `null` en vez de ausente.
 */
@Serializable
data class ScanResult(
    val subnet: String = "",
    val self: String = "",
    val gateway: String = "",
    val devices: List<Device> = emptyList(),
    val startedAt: String = "",
    /** Duración total del escaneo, en nanosegundos (como lo manda Go). */
    val duration: Long = 0L,
)

@Serializable
data class Device(
    val ip: String = "",
    val ipv6: List<String> = emptyList(),
    /** MAC normalizada "aa:bb:cc:dd:ee:ff", o "" si no se pudo resolver. */
    val mac: String = "",
    val vendor: String = "",
    val hostname: String = "",
    val identity: Identity = Identity(),
    val services: List<Service> = emptyList(),
    val openPorts: List<Int> = emptyList(),
    val sources: List<String> = emptyList(),
    val extra: Map<String, String> = emptyMap(),
    val lastSeen: String = "",
)

/**
 * Qué creemos que es el dispositivo y por qué. [reason] es la pieza de
 * transparencia clave de Vigía: se muestra siempre en el detalle, a
 * diferencia de la caja negra de apps como Fing.
 */
@Serializable
data class Identity(
    val type: String = "unknown",
    val brand: String = "",
    val model: String = "",
    val os: String = "",
    val confidence: Int = 0,
    val reason: String = "",
)

@Serializable
data class Service(
    /** mdns, ssdp o netbios. */
    val proto: String = "",
    val name: String = "",
    val info: String = "",
)
