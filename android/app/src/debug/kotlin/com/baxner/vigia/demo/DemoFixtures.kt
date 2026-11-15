package com.baxner.vigia.demo

import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.HistoryRepository
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.model.Device
import com.baxner.vigia.data.model.Identity
import com.baxner.vigia.data.model.ScanResult
import com.baxner.vigia.data.network.ChannelSpan
import com.baxner.vigia.data.network.CurrentNetwork
import com.baxner.vigia.data.network.WifiBand
import com.baxner.vigia.data.network.WifiNetworkInfo
import com.baxner.vigia.data.network.WifiSecurity
import com.baxner.vigia.data.network.WifiStandard
import com.baxner.vigia.data.network.estimatedDistanceLabel
import kotlinx.coroutines.flow.first
import java.time.Duration
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneOffset

/**
 * Datos de muestra para el modo demo (ver [DemoMode]) y para las capturas de
 * Roborazzi: la misma red "Casa" con el mismo reparto de equipos en los dos
 * casos, para no mantener dos juegos de datos separados. Todo anónimo e
 * inventado: ningún dato real de la red de nadie.
 */
object DemoFixtures {
    private const val SUBNET = "192.168.0.0/24"
    private const val SELF_IP = "192.168.0.23"
    private const val GATEWAY_IP = "192.168.0.1"

    private fun now(): String = OffsetDateTime.now(ZoneOffset.UTC).toString()

    private val router = Device(
        ip = GATEWAY_IP,
        mac = "02:00:00:10:20:01",
        vendor = "TP-Link Corporation Limited",
        hostname = "",
        identity = Identity(type = "router", brand = "Router-Lab", model = "AX3000", confidence = 95, reason = "ssdp modelo=AX3000, fabricante=TP-Link; oui TP-Link Corporation Limited"),
        openPorts = listOf(53, 80, 443),
        sources = listOf("arp", "ssdp"),
    )

    private val tv = Device(
        ip = "192.168.0.35",
        mac = "02:00:00:9b:20:44",
        vendor = "Samsung Electronics Co.,Ltd",
        hostname = "",
        identity = Identity(
            type = "tv",
            brand = "Samsung",
            model = "TV-Sala-75",
            os = "Tizen",
            confidence = 99,
            reason = "ssdp modelo=TV-Sala-75, deviceType=MediaRenderer, fabricante=Samsung Electronics; mdns _airplay._tcp; puerto 8001",
        ),
        openPorts = listOf(80, 8001, 8009),
        sources = listOf("ping", "arp", "mdns", "ssdp"),
    )

    private val imac = Device(
        ip = "192.168.0.48",
        mac = "02:00:00:12:ab:34",
        vendor = "Apple, Inc.",
        hostname = "Mac-Estudio",
        extra = mapOf("mdns:name" to "Mac-Estudio"),
        identity = Identity(
            type = "computer",
            brand = "Apple",
            model = "Mac Studio",
            os = "macOS 15 Sequoia",
            confidence = 97,
            reason = "mdns model=Mac14,13; oui Apple, Inc.",
        ),
        openPorts = listOf(22, 445),
        sources = listOf("ping", "arp", "mdns"),
    )

    private val pc = Device(
        ip = "192.168.0.4",
        mac = "02:00:00:44:51:0a",
        vendor = "Giga-Byte Technology Co.,Ltd.",
        hostname = "PC-Escritorio",
        identity = Identity(
            brand = "Gigabyte",
            type = "computer",
            os = "Windows",
            confidence = 92,
            reason = "puerto 445; puerto 139; hostname=PC-Escritorio; oui Giga-Byte Technology Co.,Ltd.",
        ),
        openPorts = listOf(139, 445, 554),
        sources = listOf("ping", "arp", "netbios"),
    )

    private val camera = Device(
        ip = "192.168.0.41",
        mac = "02:00:00:77:2e:10",
        vendor = "Hangzhou Hikvision Digital Technology Co.,Ltd.",
        hostname = "",
        identity = Identity(
            type = "camera",
            brand = "Hikvision",
            confidence = 74,
            reason = "http title=NETSurveillance WEB; puerto 554; oui Hangzhou Hikvision Digital Technology Co.,Ltd.",
        ),
        openPorts = listOf(80, 554),
        sources = listOf("ping", "arp", "api"),
    )

    private val echoShow = Device(
        ip = "192.168.0.52",
        mac = "02:00:00:aa:10:9c",
        vendor = "Amazon Technologies Inc.",
        hostname = "",
        identity = Identity(
            type = "speaker",
            brand = "Amazon",
            model = "Echo Show",
            confidence = 93,
            reason = "mdns _amzn_wplay._tcp; oui Amazon Technologies Inc.",
        ),
        openPorts = listOf(8009),
        sources = listOf("ping", "arp", "mdns", "ssdp"),
    )

    private val thisPhone = Device(
        ip = SELF_IP,
        mac = "02:00:00:88:12:34",
        hostname = "android-7f3ac19e2b4d1088",
        extra = mapOf("self" to "true"),
        identity = Identity(type = "phone", os = "Android", confidence = 80, reason = "equipo que escanea"),
        sources = listOf("self"),
    )

    /** El foco/enchufe inteligente: reconocido por el protocolo Tuya, no por puertos ni mDNS. */
    private val smartPlug = Device(
        ip = "192.168.0.76",
        mac = "02:00:00:5c:9f:22",
        vendor = "Shenzhen Tuya Technology Co., Ltd.",
        hostname = "",
        identity = Identity(
            type = "iot",
            brand = "Tuya",
            model = "Foco inteligente",
            confidence = 85,
            reason = "tuya; oui Shenzhen Tuya Technology Co., Ltd.",
        ),
        sources = listOf("ping", "icmp", "tuya"),
    )

    private val unknownWithWeakClue = Device(
        ip = "192.168.0.67",
        mac = "02:00:00:55:67:01",
        identity = Identity(type = "unknown", confidence = 30, reason = "ssdp fabricante=Shenzhen Some OEM Co.,Ltd."),
        sources = listOf("arp", "ssdp"),
    )

    private val unknown2 = Device(ip = "192.168.0.88", mac = "02:00:00:66:78:02", sources = listOf("arp"))
    private val unknown3 = Device(ip = "192.168.0.142", mac = "02:00:00:d3:e4:03", sources = listOf("arp"))
    private val unknown4 = Device(ip = "192.168.0.201", mac = "02:00:00:12:34:04", sources = listOf("ping", "arp"))

    private val baseDevices = listOf(
        router, pc, thisPhone, tv, camera, imac, echoShow, smartPlug,
        unknownWithWeakClue, unknown2, unknown3, unknown4,
    )

    /** Resultado de escaneo de muestra: siempre con `startedAt`/`lastSeen` frescos. */
    val scanResult: ScanResult
        get() {
            val startedAt = now()
            return ScanResult(
                subnet = SUBNET,
                self = SELF_IP,
                gateway = GATEWAY_IP,
                devices = baseDevices.map { it.copy(lastSeen = startedAt) },
                startedAt = startedAt,
                duration = 3_400_000_000L,
            )
        }

    /**
     * Sembrado del historial para el modo demo: varios [HistoryRepository.recordScan]
     * en el pasado (no solo el de ahora) para que el historial tenga algo que
     * contar, no solo un snapshot:
     *  - [oldPrinter] solo está en los dos escaneos viejos: termina sin
     *    conexión (con su evento [com.baxner.vigia.data.history.DeviceEventType.WENT_OFFLINE]).
     *  - [pc] cambia de IP entre el escaneo de ayer y el de ahora: evento
     *    [com.baxner.vigia.data.history.DeviceEventType.IP_CHANGED].
     *  - [echoShow] solo aparece en el escaneo de ahora: `firstSeen` reciente,
     *    la insignia "Nuevo" de Dispositivos.
     *  - [smartPlug] termina con una personalización de ejemplo ("Foco de la
     *    sala"), la misma que pide el encargo.
     */
    suspend fun seedHistory(repository: HistoryRepository, network: NetworkKey) {
        val now = Instant.now()
        val threeDaysAgo = now.minus(Duration.ofDays(3))
        val yesterday = now.minus(Duration.ofDays(1))

        val oldPrinter = Device(
            ip = "192.168.0.15",
            mac = "02:00:00:10:30:05",
            vendor = "HP Inc.",
            hostname = "",
            identity = Identity(type = "printer", brand = "HP", model = "LaserJet P1102", confidence = 88, reason = "arp; puerto 9100"),
            openPorts = listOf(9100),
            sources = listOf("arp"),
        )
        // Misma MAC que [pc], IP distinta: la reconciliación del repositorio
        // lo detecta como el mismo equipo y genera IP_CHANGED en el escaneo final.
        val pcConIpVieja = pc.copy(ip = "192.168.0.6")

        // echoShow queda fuera a propósito: solo aparece en el escaneo final.
        val equiposEstables = listOf(
            router, pcConIpVieja, thisPhone, tv, camera, imac, smartPlug,
            unknownWithWeakClue, unknown2, unknown3, unknown4,
        )

        fun scanOf(devices: List<Device>, at: Instant): ScanResult {
            val startedAt = at.toString()
            return ScanResult(
                subnet = SUBNET,
                self = SELF_IP,
                gateway = GATEWAY_IP,
                devices = devices.map { it.copy(lastSeen = startedAt) },
                startedAt = startedAt,
                duration = 3_400_000_000L,
            )
        }

        repository.recordScan(network, scanOf(equiposEstables + oldPrinter, threeDaysAgo), threeDaysAgo)
        repository.recordScan(network, scanOf(equiposEstables, yesterday), yesterday)
        repository.recordScan(network, scanResult, now)

        val known = repository.observeDevices(network).first()
        known.firstOrNull { it.mac == smartPlug.mac }?.let { foco ->
            repository.updateCustomization(
                foco.id,
                DeviceCustomization(name = "Foco de la sala", typeId = "smart_bulb", location = "Sala", important = true),
            )
        }
    }

    private val casaBssid = "02:00:00:10:20:5a"
    private const val ROUTER_VENDOR = "TP-Link" // lo que da Mobile.brandForMAC

    /**
     * "Casa" es una red mesh de 3 puntos de acceso (mismo router Router-Lab AX3000 de
     * [router] arriba, de ahí Wi-Fi 6 en los tres): el nodo conectado en 5 GHz,
     * un segundo nodo 5 GHz más lejos (-60 dBm, como el ejemplo de WiFi
     * Analyzer que pidió el encargo) y el nodo 2.4 GHz. Entre los 9 total
     * cubren los 4 anchos de canal con centro distinto del canal principal
     * (20/40/80/160 MHz), BSSID+estándar+WPS variados y una red oculta.
     */
    val wifiNetworks: List<WifiNetworkInfo> = listOf(
        WifiNetworkInfo(
            ssid = "Casa", bssid = casaBssid, rssiDbm = -42, frequencyMhz = 5220,
            band = WifiBand.GHZ_5, channel = 44, channelWidthLabel = "80 MHz",
            channelSpan = ChannelSpan(centerChannel = 42, startMhz = 5170, endMhz = 5250),
            wifiStandard = WifiStandard.WIFI_6,
            security = WifiSecurity.WPA2, hasWps = true, vendor = ROUTER_VENDOR,
            distanceLabel = estimatedDistanceLabel(5220, -42), isConnected = true,
        ),
        WifiNetworkInfo(
            ssid = "Casa", bssid = "02:00:00:10:20:5c", rssiDbm = -60, frequencyMhz = 5745,
            band = WifiBand.GHZ_5, channel = 149, channelWidthLabel = "80 MHz",
            channelSpan = ChannelSpan(centerChannel = 155, startMhz = 5735, endMhz = 5815),
            wifiStandard = WifiStandard.WIFI_6,
            security = WifiSecurity.WPA2, hasWps = false, vendor = ROUTER_VENDOR,
            distanceLabel = estimatedDistanceLabel(5745, -60), isConnected = false,
        ),
        WifiNetworkInfo(
            ssid = "Casa", bssid = "02:00:00:10:20:24", rssiDbm = -55, frequencyMhz = 2437,
            band = WifiBand.GHZ_2_4, channel = 6, channelWidthLabel = "40 MHz",
            channelSpan = ChannelSpan(centerChannel = 4, startMhz = 2407, endMhz = 2447),
            wifiStandard = WifiStandard.WIFI_6,
            security = WifiSecurity.WPA2, hasWps = false, vendor = ROUTER_VENDOR,
            distanceLabel = estimatedDistanceLabel(2437, -55), isConnected = false,
        ),
        WifiNetworkInfo(
            ssid = "INFINITUM_1234", bssid = "02:00:00:11:22:33", rssiDbm = -80, frequencyMhz = 2412,
            band = WifiBand.GHZ_2_4, channel = 1, channelWidthLabel = "20 MHz",
            wifiStandard = WifiStandard.WIFI_4,
            security = WifiSecurity.OPEN, hasWps = true, vendor = "Huawei",
            distanceLabel = estimatedDistanceLabel(2412, -80), isConnected = false,
        ),
        WifiNetworkInfo(
            ssid = "CasaVecina", bssid = "02:00:00:33:44:55", rssiDbm = -85, frequencyMhz = 2462,
            band = WifiBand.GHZ_2_4, channel = 11, channelWidthLabel = "20 MHz",
            security = WifiSecurity.WEP, hasWps = false, vendor = "",
            distanceLabel = estimatedDistanceLabel(2462, -85), isConnected = false,
        ),
        WifiNetworkInfo(
            ssid = "MiFi-Repetidor", bssid = "02:00:00:66:77:88", rssiDbm = -70, frequencyMhz = 2437,
            band = WifiBand.GHZ_2_4, channel = 6, channelWidthLabel = "20 MHz",
            wifiStandard = WifiStandard.WIFI_4,
            security = WifiSecurity.WPA, hasWps = true, vendor = "",
            distanceLabel = estimatedDistanceLabel(2437, -70), isConnected = false,
        ),
        WifiNetworkInfo(
            ssid = "ATT-5G-Guest", bssid = "02:00:00:99:00:11", rssiDbm = -75, frequencyMhz = 5180,
            band = WifiBand.GHZ_5, channel = 36, channelWidthLabel = "80 MHz",
            channelSpan = ChannelSpan(centerChannel = 42, startMhz = 5170, endMhz = 5250),
            wifiStandard = WifiStandard.WIFI_5,
            security = WifiSecurity.WPA2, hasWps = false, vendor = "",
            distanceLabel = estimatedDistanceLabel(5180, -75), isConnected = false,
        ),
        // Mismo canal/centro/ancho que el ejemplo del encargo: CH 48(50), 160 MHz, 5170-5330.
        WifiNetworkInfo(
            ssid = "Xfinity", bssid = "02:00:00:22:33:44", rssiDbm = -68, frequencyMhz = 5240,
            band = WifiBand.GHZ_5, channel = 48, channelWidthLabel = "160 MHz",
            channelSpan = ChannelSpan(centerChannel = 50, startMhz = 5170, endMhz = 5330),
            wifiStandard = WifiStandard.WIFI_6,
            security = WifiSecurity.WPA3, hasWps = false, vendor = "",
            distanceLabel = estimatedDistanceLabel(5240, -68), isConnected = false,
        ),
        WifiNetworkInfo(
            ssid = "", bssid = "02:00:00:55:66:77", rssiDbm = -90, frequencyMhz = 2462,
            band = WifiBand.GHZ_2_4, channel = 11, channelWidthLabel = "20 MHz",
            wifiStandard = WifiStandard.WIFI_4,
            security = WifiSecurity.WPA2, hasWps = false, vendor = "",
            distanceLabel = estimatedDistanceLabel(2462, -90), isConnected = false,
        ),
    )

    val currentNetwork: CurrentNetwork
        get() = CurrentNetwork(
            cidr = "$SELF_IP/24",
            gateway = GATEWAY_IP,
            isWifi = true,
            ssid = "Casa",
            bssid = casaBssid,
            rssi = -42,
            frequencyMhz = 5220,
            linkSpeedMbps = 866,
            isEthernet = false,
            vpnActive = false,
            network = null,
        )
}
