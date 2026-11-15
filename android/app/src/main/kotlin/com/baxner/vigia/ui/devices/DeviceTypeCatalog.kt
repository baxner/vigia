package com.baxner.vigia.ui.devices

import androidx.annotation.StringRes
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Dvr
import androidx.compose.material.icons.automirrored.filled.MenuBook
import androidx.compose.material.icons.filled.AcUnit
import androidx.compose.material.icons.filled.Cast
import androidx.compose.material.icons.filled.Category
import androidx.compose.material.icons.filled.CleaningServices
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.DesktopWindows
import androidx.compose.material.icons.filled.DirectionsCar
import androidx.compose.material.icons.filled.Doorbell
import androidx.compose.material.icons.filled.Hub
import androidx.compose.material.icons.filled.Kitchen
import androidx.compose.material.icons.filled.Lan
import androidx.compose.material.icons.filled.Laptop
import androidx.compose.material.icons.filled.Light
import androidx.compose.material.icons.filled.Lightbulb
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.NotificationsActive
import androidx.compose.material.icons.filled.Outlet
import androidx.compose.material.icons.filled.Print
import androidx.compose.material.icons.filled.Router
import androidx.compose.material.icons.filled.Scanner
import androidx.compose.material.icons.filled.Sensors
import androidx.compose.material.icons.filled.Smartphone
import androidx.compose.material.icons.filled.Speaker
import androidx.compose.material.icons.filled.SpeakerGroup
import androidx.compose.material.icons.filled.SportsEsports
import androidx.compose.material.icons.filled.Storage
import androidx.compose.material.icons.filled.Tablet
import androidx.compose.material.icons.filled.Theaters
import androidx.compose.material.icons.filled.Thermostat
import androidx.compose.material.icons.filled.ToggleOn
import androidx.compose.material.icons.filled.Tv
import androidx.compose.material.icons.filled.Videocam
import androidx.compose.material.icons.filled.Watch
import androidx.compose.material.icons.filled.Wifi
import androidx.compose.material.icons.filled.WifiTethering
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import com.baxner.vigia.R
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.DeviceType
import com.baxner.vigia.data.model.displayName
import com.baxner.vigia.data.model.nameOrNull
import java.time.Duration
import java.time.Instant

/** Una categoría del catálogo de tipos de equipo (pantalla Selector de tipo). */
data class DeviceTypeCategory(val id: String, @StringRes val labelRes: Int)

@Composable
fun DeviceTypeCategory.label(): String = stringResource(labelRes)

/**
 * Un tipo de equipo del catálogo de Editar/Selector de tipo: más fino que
 * [DeviceType] (p. ej. distingue "Foco" de "Enchufe", los dos [DeviceType.IOT]).
 * [id] es lo que se guarda en [com.baxner.vigia.data.history.DeviceCustomization.typeId];
 * [coarse] es el tipo "grueso" al que equivale para todo lo que ya entiende
 * solo esas 13 categorías (radar, filtros, conteos por tipo).
 *
 * Vive en `ui` y no en `data/model` porque es puramente de la personalización
 * manual: el motor nunca manda estos ids, solo un [DeviceType.raw].
 */
data class CatalogDeviceType(
    val id: String,
    val categoryId: String,
    @StringRes val labelRes: Int,
    val icon: ImageVector,
    val coarse: DeviceType,
)

@Composable
fun CatalogDeviceType.label(): String = stringResource(labelRes)

/** Catálogo de tipos de equipo de la pantalla Editar, por categorías, como Fing pero con más detalle. */
object DeviceTypeCatalog {
    val categories: List<DeviceTypeCategory> = listOf(
        DeviceTypeCategory("mobile", R.string.device_cat_mobile),
        DeviceTypeCategory("computers", R.string.device_cat_computers),
        DeviceTypeCategory("av", R.string.device_cat_av),
        DeviceTypeCategory("smart_home", R.string.device_cat_smart_home),
        DeviceTypeCategory("security", R.string.device_cat_security),
        DeviceTypeCategory("network", R.string.device_cat_network),
        DeviceTypeCategory("office", R.string.device_cat_office),
        DeviceTypeCategory("other", R.string.device_cat_other),
    )

    val entries: List<CatalogDeviceType> = listOf(
        // Móviles
        CatalogDeviceType("phone", "mobile", R.string.device_catalog_phone, Icons.Default.Smartphone, DeviceType.PHONE),
        CatalogDeviceType("tablet", "mobile", R.string.device_catalog_tablet, Icons.Default.Tablet, DeviceType.TABLET),
        CatalogDeviceType("watch", "mobile", R.string.device_catalog_watch, Icons.Default.Watch, DeviceType.WEARABLE),
        CatalogDeviceType("ereader", "mobile", R.string.device_catalog_ereader, Icons.AutoMirrored.Filled.MenuBook, DeviceType.TABLET),
        CatalogDeviceType("car", "mobile", R.string.device_catalog_car, Icons.Default.DirectionsCar, DeviceType.IOT),

        // Computadoras
        CatalogDeviceType("laptop", "computers", R.string.device_catalog_laptop, Icons.Default.Laptop, DeviceType.COMPUTER),
        CatalogDeviceType("desktop", "computers", R.string.device_catalog_desktop, Icons.Default.DesktopWindows, DeviceType.COMPUTER),
        CatalogDeviceType("server", "computers", R.string.device_catalog_server, Icons.Default.Dns, DeviceType.COMPUTER),
        CatalogDeviceType("raspberry_pi", "computers", R.string.device_catalog_raspberry_pi, Icons.Default.Memory, DeviceType.COMPUTER),

        // Audio y video
        CatalogDeviceType("tv", "av", R.string.device_catalog_tv, Icons.Default.Tv, DeviceType.TV),
        CatalogDeviceType("streaming_dongle", "av", R.string.device_catalog_streaming_dongle, Icons.Default.Cast, DeviceType.TV),
        CatalogDeviceType("smart_speaker", "av", R.string.device_catalog_smart_speaker, Icons.Default.Speaker, DeviceType.SPEAKER),
        CatalogDeviceType("soundbar", "av", R.string.device_catalog_soundbar, Icons.Default.SpeakerGroup, DeviceType.SPEAKER),
        CatalogDeviceType("console", "av", R.string.device_catalog_console, Icons.Default.SportsEsports, DeviceType.CONSOLE),
        CatalogDeviceType("projector", "av", R.string.device_catalog_projector, Icons.Default.Theaters, DeviceType.TV),

        // Hogar inteligente
        CatalogDeviceType("smart_bulb", "smart_home", R.string.device_catalog_smart_bulb, Icons.Default.Lightbulb, DeviceType.IOT),
        CatalogDeviceType("led_strip", "smart_home", R.string.device_catalog_led_strip, Icons.Default.Light, DeviceType.IOT),
        CatalogDeviceType("smart_plug", "smart_home", R.string.device_catalog_smart_plug, Icons.Default.Outlet, DeviceType.IOT),
        CatalogDeviceType("smart_switch", "smart_home", R.string.device_catalog_smart_switch, Icons.Default.ToggleOn, DeviceType.IOT),
        CatalogDeviceType("sensor", "smart_home", R.string.device_catalog_sensor, Icons.Default.Sensors, DeviceType.IOT),
        CatalogDeviceType("hub", "smart_home", R.string.device_catalog_hub, Icons.Default.Hub, DeviceType.IOT),
        CatalogDeviceType("thermostat", "smart_home", R.string.device_catalog_thermostat, Icons.Default.Thermostat, DeviceType.IOT),
        CatalogDeviceType("vacuum", "smart_home", R.string.device_catalog_vacuum, Icons.Default.CleaningServices, DeviceType.IOT),
        CatalogDeviceType("appliance", "smart_home", R.string.device_catalog_appliance, Icons.Default.Kitchen, DeviceType.IOT),
        CatalogDeviceType("ac", "smart_home", R.string.device_catalog_ac, Icons.Default.AcUnit, DeviceType.IOT),

        // Seguridad
        CatalogDeviceType("camera", "security", R.string.device_catalog_camera, Icons.Default.Videocam, DeviceType.CAMERA),
        CatalogDeviceType("doorbell", "security", R.string.device_catalog_doorbell, Icons.Default.Doorbell, DeviceType.CAMERA),
        CatalogDeviceType("nvr", "security", R.string.device_catalog_nvr, Icons.AutoMirrored.Filled.Dvr, DeviceType.CAMERA),
        CatalogDeviceType("alarm", "security", R.string.device_catalog_alarm, Icons.Default.NotificationsActive, DeviceType.IOT),

        // Redes
        CatalogDeviceType("router", "network", R.string.device_catalog_router, Icons.Default.Router, DeviceType.ROUTER),
        CatalogDeviceType("mesh_repeater", "network", R.string.device_catalog_mesh_repeater, Icons.Default.Wifi, DeviceType.ROUTER),
        CatalogDeviceType("switch", "network", R.string.device_catalog_switch, Icons.Default.Lan, DeviceType.ROUTER),
        CatalogDeviceType("access_point", "network", R.string.device_catalog_access_point, Icons.Default.WifiTethering, DeviceType.ROUTER),

        // Oficina
        CatalogDeviceType("printer", "office", R.string.device_catalog_printer, Icons.Default.Print, DeviceType.PRINTER),
        CatalogDeviceType("nas", "office", R.string.device_catalog_nas, Icons.Default.Storage, DeviceType.NAS),
        CatalogDeviceType("scanner", "office", R.string.device_catalog_scanner, Icons.Default.Scanner, DeviceType.PRINTER),

        // Otro
        CatalogDeviceType("other", "other", R.string.device_catalog_other, Icons.Default.Category, DeviceType.UNKNOWN),
    )

    private val byId = entries.associateBy { it.id }

    /** La entrada del catálogo de [id], o null si es "Automático" (id nulo o desconocido). */
    fun find(id: String?): CatalogDeviceType? = id?.let { byId[it] }

    fun entriesFor(categoryId: String): List<CatalogDeviceType> = entries.filter { it.categoryId == categoryId }
}

// ---- Resolución de personalización ----
// El nombre y el tipo que elige el usuario mandan sobre los que detectó el
// motor, en toda la app (lista, radar, Panel, detalle): estas son las únicas
// funciones que deberían leer KnownDevice.custom directamente para decidir
// qué nombre/icono/tipo mostrar.

/** Entrada del catálogo elegida a mano, o null si el tipo sigue en automático. */
fun KnownDevice.catalogType(): CatalogDeviceType? = DeviceTypeCatalog.find(custom.typeId)

/** Tipo "grueso" del equipo: el de su tipo elegido a mano, o si no el que detectó el motor. */
fun KnownDevice.coarseType(): DeviceType = catalogType()?.coarse ?: DeviceType.fromRaw(last.identity.type)

/** Icono del equipo: el de su tipo elegido a mano, o si no el del tipo que detectó el motor. */
fun KnownDevice.icon(): ImageVector = catalogType()?.icon ?: coarseType().icon()

/** Etiqueta del tipo a mostrar: la elegida a mano, o si no la que detectó el motor. */
@Composable
fun KnownDevice.typeLabel(): String = catalogType()?.label() ?: coarseType().label()

/** Nombre a mostrar: el que puso el usuario, o si no el que calculó el motor (o la IP). */
fun KnownDevice.displayName(): String = custom.name?.trim()?.takeUnless { it.isEmpty() } ?: last.displayName()

/** El nombre propio (personalizado o el que dedujo el motor), o null si solo hay IP. */
fun KnownDevice.nameOrNull(): String? = custom.name?.trim()?.takeUnless { it.isEmpty() } ?: last.nameOrNull()

/** Mismo criterio de color que [com.baxner.vigia.ui.devices.statusColor] pero respetando el tipo elegido a mano. */
fun KnownDevice.statusColor(): Color = statusColorFor(last, coarseType())

/**
 * Insignia "Nuevo": el equipo apareció hace menos de 24 h Y después de que la
 * red ya se estaba vigilando. [baseline] es el primer escaneo de la red (el
 * firstSeen más antiguo de sus equipos): lo que estaba ahí desde el principio
 * es la línea base, no un equipo nuevo — si no, tras el primer escaneo todo
 * salía "Nuevo" durante un día.
 */
fun KnownDevice.isNew(now: Instant = Instant.now(), baseline: Instant? = null): Boolean {
    val age = Duration.between(firstSeen, now)
    if (age.isNegative || age >= Duration.ofHours(24)) return false
    return baseline == null || Duration.between(baseline, firstSeen) > NEW_AFTER_BASELINE
}

/** Margen tras el primer escaneo: los equipos que aparecen en esos minutos también son línea base. */
private val NEW_AFTER_BASELINE: Duration = Duration.ofMinutes(15)

/** El primer escaneo de una red, según sus equipos conocidos. */
fun baselineOf(devices: List<KnownDevice>): Instant? = devices.minOfOrNull { it.firstSeen }

// ---- Lo mismo, pero para el Panel ----
// El Panel (radar, "Por tipo", "Atención") sigue dibujando directo sobre el
// último [Device] del escaneo, no sobre [KnownDevice]: le basta un mapa
// IP -> personalización (solo de los equipos que de verdad tienen algo
// personalizado) para respetar la misma regla sin tener que re-resolver la
// red actual ahí. Ver [com.baxner.vigia.ui.panel.PanelScreen].

/** Tipo "grueso" de [device]: el de [customByIp] si lo personalizaron, si no el que detectó el motor. */
fun Device.effectiveCoarseType(customByIp: Map<String, DeviceCustomization>): DeviceType =
    customByIp[ip]?.typeId?.let { DeviceTypeCatalog.find(it)?.coarse } ?: DeviceType.fromRaw(identity.type)

/** Nombre a mostrar de [device] respetando [customByIp]. */
fun Device.effectiveDisplayName(customByIp: Map<String, DeviceCustomization>): String =
    customByIp[ip]?.name?.trim()?.takeUnless { it.isEmpty() } ?: displayName()
