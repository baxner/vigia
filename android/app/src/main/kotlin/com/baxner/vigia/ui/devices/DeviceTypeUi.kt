package com.baxner.vigia.ui.devices

import androidx.annotation.StringRes
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Computer
import androidx.compose.material.icons.filled.DeviceUnknown
import androidx.compose.material.icons.filled.Print
import androidx.compose.material.icons.filled.Router
import androidx.compose.material.icons.filled.Sensors
import androidx.compose.material.icons.filled.Smartphone
import androidx.compose.material.icons.filled.Speaker
import androidx.compose.material.icons.filled.SportsEsports
import androidx.compose.material.icons.filled.Storage
import androidx.compose.material.icons.filled.Tablet
import androidx.compose.material.icons.filled.Tv
import androidx.compose.material.icons.filled.Videocam
import androidx.compose.material.icons.filled.Watch
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import com.baxner.vigia.R
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.DeviceType
import com.baxner.vigia.data.model.PortCatalog
import com.baxner.vigia.ui.theme.VigiaColors

/** Icono de Material Icons para cada [DeviceType]. Vive en `ui` y no en
 * `data` porque `ImageVector` es un tipo de Compose, no de dominio. */
fun DeviceType.icon(): ImageVector = when (this) {
    DeviceType.PHONE -> Icons.Default.Smartphone
    DeviceType.COMPUTER -> Icons.Default.Computer
    DeviceType.TV -> Icons.Default.Tv
    DeviceType.ROUTER -> Icons.Default.Router
    DeviceType.PRINTER -> Icons.Default.Print
    DeviceType.CAMERA -> Icons.Default.Videocam
    DeviceType.SPEAKER -> Icons.Default.Speaker
    DeviceType.CONSOLE -> Icons.Default.SportsEsports
    DeviceType.NAS -> Icons.Default.Storage
    DeviceType.IOT -> Icons.Default.Sensors
    DeviceType.WEARABLE -> Icons.Default.Watch
    DeviceType.TABLET -> Icons.Default.Tablet
    DeviceType.UNKNOWN -> Icons.Default.DeviceUnknown
}

/** Recurso de texto localizado para cada [DeviceType]. */
@StringRes
fun DeviceType.labelRes(): Int = when (this) {
    DeviceType.PHONE -> R.string.device_type_phone
    DeviceType.COMPUTER -> R.string.device_type_computer
    DeviceType.TV -> R.string.device_type_tv
    DeviceType.ROUTER -> R.string.device_type_router
    DeviceType.PRINTER -> R.string.device_type_printer
    DeviceType.CAMERA -> R.string.device_type_camera
    DeviceType.SPEAKER -> R.string.device_type_speaker
    DeviceType.CONSOLE -> R.string.device_type_console
    DeviceType.NAS -> R.string.device_type_nas
    DeviceType.IOT -> R.string.device_type_iot
    DeviceType.WEARABLE -> R.string.device_type_wearable
    DeviceType.TABLET -> R.string.device_type_tablet
    DeviceType.UNKNOWN -> R.string.device_type_unknown
}

/** Etiqueta localizada para cada [DeviceType]. */
@Composable
fun DeviceType.label(): String = stringResource(labelRes())

/**
 * Anillo del radar del Panel para cada [DeviceType]: el router al centro
 * (0), computadoras/teléfonos/tablets cerca (1), TV/bocinas/consolas/
 * impresoras/NAS a media distancia (2) y cámaras/IoT/wearables/sin
 * identificar hacia afuera (3).
 */
fun DeviceType.radarRing(): Int = when (this) {
    DeviceType.ROUTER -> 0
    DeviceType.COMPUTER, DeviceType.PHONE, DeviceType.TABLET -> 1
    DeviceType.TV, DeviceType.SPEAKER, DeviceType.CONSOLE, DeviceType.PRINTER, DeviceType.NAS -> 2
    DeviceType.CAMERA, DeviceType.IOT, DeviceType.WEARABLE, DeviceType.UNKNOWN -> 3
}

/**
 * El color de estado de un equipo, igual en el punto del radar y en su fila
 * de Dispositivos: rojo si tiene algún puerto de riesgo abierto (gana sobre
 * lo demás: es lo más urgente), ámbar si no se pudo identificar, cian si
 * todo está en orden.
 */
fun Device.statusColor(): Color = statusColorFor(this, DeviceType.fromRaw(identity.type))

/**
 * El mismo criterio que [Device.statusColor] pero con el [type] ya resuelto
 * aparte: así el Panel puede pintar un equipo personalizado (tipo elegido a
 * mano, ver [com.baxner.vigia.ui.devices.coarseType]) sin duplicar la regla.
 */
fun statusColorFor(device: Device, type: DeviceType): Color {
    val risky = PortCatalog.riskyAmong(device.openPorts).isNotEmpty()
    return when {
        risky -> VigiaColors.Risk
        type == DeviceType.UNKNOWN -> VigiaColors.Amber
        else -> VigiaColors.RadarCyan
    }
}

/**
 * Posición angular (0–360°) de cada equipo en el radar del Panel. Puramente
 * visual: dentro de cada anillo los equipos se ordenan por IP y se reparten
 * por igual, con un desfase por anillo (ángulo áureo) para que no queden
 * alineados. Antes salía del último octeto de la IP y, como en una casa casi
 * todas son bajas (.1–.150), se amontonaban en un cuarto del círculo.
 */
fun radarAngles(devices: List<Device>, ringOf: (Device) -> Int = { DeviceType.fromRaw(it.identity.type).radarRing() }): Map<String, Float> {
    val out = HashMap<String, Float>(devices.size)
    devices.groupBy(ringOf).forEach { (ring, inRing) ->
        val sorted = inRing.sortedBy { ipSortKey(it.ip) }
        val step = 360f / sorted.size
        val offset = (ring * 137.5f) % 360f
        sorted.forEachIndexed { i, d -> out[d.ip] = (offset + i * step) % 360f }
    }
    return out
}

private fun ipSortKey(ip: String): Long =
    ip.split('.').fold(0L) { acc, part -> acc * 256 + (part.toLongOrNull() ?: 0L) }

/** Fuentes del motor ("arp", "mdns"…) con nombres localizados que dicen algo. */
@Composable
fun sourceLabel(source: String): String = when (source) {
    "ping" -> "TCP"
    "icmp" -> "ping"
    "arp" -> stringResource(R.string.device_source_arp)
    "ndp" -> stringResource(R.string.device_source_ndp)
    "mdns" -> "Bonjour"
    "ssdp" -> "UPnP"
    "netbios" -> "NetBIOS"
    "tuya" -> "Tuya"
    "api" -> stringResource(R.string.device_source_api)
    "self" -> stringResource(R.string.device_source_self)
    else -> source
}
