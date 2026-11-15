package com.baxner.vigia.data.network

import android.net.wifi.ScanResult
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class WifiUtilsTest {

    // --- frecuencia -> banda/canal ---

    @Test
    fun `2_4GHz canal 1`() {
        val info = channelInfoForFrequencyMhz(2412)
        assertEquals(WifiBand.GHZ_2_4, info.band)
        assertEquals(1, info.channel)
    }

    @Test
    fun `2_4GHz canal 13, el ultimo de la grilla normal`() {
        val info = channelInfoForFrequencyMhz(2472)
        assertEquals(WifiBand.GHZ_2_4, info.band)
        assertEquals(13, info.channel)
    }

    @Test
    fun `2_4GHz canal 14 (Japon, fuera de la grilla de 5 MHz)`() {
        val info = channelInfoForFrequencyMhz(2484)
        assertEquals(WifiBand.GHZ_2_4, info.band)
        assertEquals(14, info.channel)
    }

    @Test
    fun `5GHz canal 36`() {
        val info = channelInfoForFrequencyMhz(5180)
        assertEquals(WifiBand.GHZ_5, info.band)
        assertEquals(36, info.channel)
    }

    @Test
    fun `5GHz canal 165`() {
        val info = channelInfoForFrequencyMhz(5825)
        assertEquals(WifiBand.GHZ_5, info.band)
        assertEquals(165, info.channel)
    }

    @Test
    fun `6GHz canal 1`() {
        val info = channelInfoForFrequencyMhz(5955)
        assertEquals(WifiBand.GHZ_6, info.band)
        assertEquals(1, info.channel)
    }

    @Test
    fun `6GHz canal 233, el ultimo`() {
        val info = channelInfoForFrequencyMhz(7115)
        assertEquals(WifiBand.GHZ_6, info.band)
        assertEquals(233, info.channel)
    }

    @Test
    fun `frecuencia fuera de cualquier banda conocida`() {
        val info = channelInfoForFrequencyMhz(1234)
        assertEquals(WifiBand.UNKNOWN, info.band)
        assertEquals(-1, info.channel)
    }

    // --- capabilities -> seguridad ---

    @Test
    fun `red abierta (solo ESS, sin tokens de seguridad)`() {
        assertEquals(WifiSecurity.OPEN, securityFromCapabilities("[ESS]"))
    }

    @Test
    fun `WEP`() {
        assertEquals(WifiSecurity.WEP, securityFromCapabilities("[WEP][ESS]"))
    }

    @Test
    fun `WPA simple`() {
        assertEquals(WifiSecurity.WPA, securityFromCapabilities("[WPA-PSK-TKIP][ESS]"))
    }

    @Test
    fun `WPA2 via token WPA2`() {
        assertEquals(WifiSecurity.WPA2, securityFromCapabilities("[WPA2-PSK-CCMP][ESS]"))
    }

    @Test
    fun `WPA2 via token RSN`() {
        assertEquals(WifiSecurity.WPA2, securityFromCapabilities("[RSN-PSK-CCMP][ESS]"))
    }

    @Test
    fun `WPA3 via SAE`() {
        assertEquals(WifiSecurity.WPA3, securityFromCapabilities("[RSN-SAE-CCMP][ESS]"))
    }

    @Test
    fun `modo transicion WPA2-WPA3 se reporta como WPA3 (la mas fuerte)`() {
        assertEquals(WifiSecurity.WPA3, securityFromCapabilities("[RSN-PSK+SAE-CCMP][ESS]"))
    }

    @Test
    fun `capabilities vacio es Abierta`() {
        assertEquals(WifiSecurity.OPEN, securityFromCapabilities(""))
    }

    // --- ancho de canal (constantes reales de android.net.wifi.ScanResult) ---

    @Test
    fun `ancho de canal 20MHz`() {
        assertEquals("20 MHz", channelWidthLabel(ScanResult.CHANNEL_WIDTH_20MHZ))
    }

    @Test
    fun `ancho de canal 80MHz`() {
        assertEquals("80 MHz", channelWidthLabel(ScanResult.CHANNEL_WIDTH_80MHZ))
    }

    @Test
    fun `ancho de canal desconocido`() {
        assertEquals("? MHz", channelWidthLabel(-1))
    }

    // --- rssi -> barras ---

    @Test
    fun `rssi excelente son 4 barras`() {
        assertEquals(4, rssiToBars(-40))
    }

    @Test
    fun `rssi muy debil son 0 barras`() {
        assertEquals(0, rssiToBars(-95))
    }

    @Test
    fun `rssi en el borde de un umbral`() {
        assertEquals(3, rssiToBars(-66))
        assertEquals(2, rssiToBars(-67))
    }

    @Test
    fun `vendorForBssid usa la BSSID tal cual si tiene fabricante`() {
        val vendor = vendorForBssid("00:11:22:33:44:50") { if (it == "00:11:22:33:44:50") "TP-Link" else "" }
        assertEquals("TP-Link", vendor)
    }

    @Test
    fun `vendorForBssid apaga el bit local de una BSSID virtual`() {
        // 02 = 00 con el bit 0x02 encendido (red de invitados / mesh).
        val vendor = vendorForBssid("02:11:22:33:44:51") { if (it == "00:11:22:33:44:51") "TP-Link" else "" }
        assertEquals("TP-Link", vendor)
    }

    @Test
    fun `vendorForBssid no inventa fabricante para MAC global desconocida ni basura`() {
        assertEquals("", vendorForBssid("00:11:22:33:44:55") { "" })
        assertEquals("", vendorForBssid("") { "X" })
        assertEquals("", vendorForBssid("zz") { "" })
        assertEquals("", vendorForBssid("aabbccddeeff") { "" })
    }

    // --- wifiStandard -> generación Wi-Fi ---

    @Test
    fun `11N es Wi-Fi 4`() {
        assertEquals(WifiStandard.WIFI_4, wifiStandardFor(ScanResult.WIFI_STANDARD_11N, WifiBand.GHZ_2_4))
    }

    @Test
    fun `11AC es Wi-Fi 5`() {
        assertEquals(WifiStandard.WIFI_5, wifiStandardFor(ScanResult.WIFI_STANDARD_11AC, WifiBand.GHZ_5))
    }

    @Test
    fun `11AX en 5GHz es Wi-Fi 6`() {
        assertEquals(WifiStandard.WIFI_6, wifiStandardFor(ScanResult.WIFI_STANDARD_11AX, WifiBand.GHZ_5))
    }

    @Test
    fun `11AX en 6GHz es Wi-Fi 6E`() {
        assertEquals(WifiStandard.WIFI_6E, wifiStandardFor(ScanResult.WIFI_STANDARD_11AX, WifiBand.GHZ_6))
    }

    @Test
    fun `11BE es Wi-Fi 7`() {
        assertEquals(WifiStandard.WIFI_7, wifiStandardFor(ScanResult.WIFI_STANDARD_11BE, WifiBand.GHZ_6))
    }

    @Test
    fun `legacy y desconocido no mapean a ningun estandar`() {
        assertNull(wifiStandardFor(ScanResult.WIFI_STANDARD_LEGACY, WifiBand.GHZ_2_4))
        assertNull(wifiStandardFor(ScanResult.WIFI_STANDARD_UNKNOWN, WifiBand.GHZ_2_4))
    }

    @Test
    fun `11AD (WiGig) tampoco mapea, queda fuera de la numeracion Wi-Fi N`() {
        assertNull(wifiStandardFor(ScanResult.WIFI_STANDARD_11AD, WifiBand.GHZ_5))
    }

    @Test
    fun `valor centinela de API menor a 30 no mapea`() {
        assertNull(wifiStandardFor(-1, WifiBand.GHZ_5))
    }

    // --- canal central y rango (centerFreq0 + channelWidth) ---

    @Test
    fun `160 MHz con centro 5250 da canal central 50 y rango 5170-5330`() {
        val span = channelSpanFor(centerFreq0Mhz = 5250, channelWidth = ScanResult.CHANNEL_WIDTH_160MHZ)
        assertEquals(ChannelSpan(centerChannel = 50, startMhz = 5170, endMhz = 5330), span)
    }

    @Test
    fun `80 MHz con centro 5210 da canal central 42 y rango 5170-5250`() {
        val span = channelSpanFor(centerFreq0Mhz = 5210, channelWidth = ScanResult.CHANNEL_WIDTH_80MHZ)
        assertEquals(ChannelSpan(centerChannel = 42, startMhz = 5170, endMhz = 5250), span)
    }

    @Test
    fun `40 MHz con centro 2427 da canal central 4 y rango 2407-2447`() {
        val span = channelSpanFor(centerFreq0Mhz = 2427, channelWidth = ScanResult.CHANNEL_WIDTH_40MHZ)
        assertEquals(ChannelSpan(centerChannel = 4, startMhz = 2407, endMhz = 2447), span)
    }

    @Test
    fun `320 MHz calcula el rango mas ancho`() {
        val span = channelSpanFor(centerFreq0Mhz = 6105, channelWidth = ScanResult.CHANNEL_WIDTH_320MHZ)
        assertEquals(ChannelSpan(centerChannel = 31, startMhz = 5945, endMhz = 6265), span)
    }

    @Test
    fun `20 MHz no tiene centro distinto, da null`() {
        assertNull(channelSpanFor(centerFreq0Mhz = 5240, channelWidth = ScanResult.CHANNEL_WIDTH_20MHZ))
    }

    @Test
    fun `80+80 MHz no contiguo queda fuera a proposito, da null`() {
        assertNull(channelSpanFor(centerFreq0Mhz = 5210, channelWidth = ScanResult.CHANNEL_WIDTH_80MHZ_PLUS_MHZ))
    }

    @Test
    fun `sin centerFreq0 valido da null`() {
        assertNull(channelSpanFor(centerFreq0Mhz = 0, channelWidth = ScanResult.CHANNEL_WIDTH_160MHZ))
    }

    // --- WPS ---

    @Test
    fun `hasWps detecta el token WPS en capabilities`() {
        assertTrue(hasWps("[WPS][WPA2-PSK-CCMP][ESS]"))
    }

    @Test
    fun `hasWps es insensible a mayusculas`() {
        assertTrue(hasWps("[wps][wpa2-psk-ccmp][ess]"))
    }

    @Test
    fun `hasWps es false sin el token`() {
        assertFalse(hasWps("[WPA2-PSK-CCMP][ESS]"))
        assertFalse(hasWps(""))
    }

    // --- distancia estimada (perdida en espacio libre) ---

    @Test
    fun `distancia tipica de WiFi Analyzer, -21dBm a 5240MHz, da ~0,1 m`() {
        assertEquals("~0.1 m", estimatedDistanceLabel(frequencyMhz = 5240, rssiDbm = -21))
    }

    @Test
    fun `senal debil a 2_4GHz da una distancia de varios metros`() {
        // -80 dBm a 2437 MHz: lejos, se redondea a metros enteros (sin decimales).
        val label = estimatedDistanceLabel(frequencyMhz = 2437, rssiDbm = -80)
        assertTrue("se esperaba un entero de metros, fue $label", label.matches(Regex("~\\d+ m")))
    }

    @Test
    fun `formatDistanceMeters redondea bajo 1 metro con un decimal`() {
        assertEquals("~0.1 m", formatDistanceMeters(0.051))
    }

    @Test
    fun `formatDistanceMeters redondea desde 1 metro sin decimales`() {
        assertEquals("~3 m", formatDistanceMeters(2.98))
        assertEquals("~14 m", formatDistanceMeters(14.2))
    }

    @Test
    fun `formatDistanceMeters cruza el umbral de 1 metro tras redondear a decimas`() {
        // 0.96 redondeado a decimas es 1.0: ya no debe mostrar el decimal.
        assertEquals("~1 m", formatDistanceMeters(0.96))
    }

    @Test
    fun `frecuencia invalida da distancia 0`() {
        assertEquals(0.0, estimatedDistanceMeters(frequencyMhz = 0, rssiDbm = -50), 0.0001)
    }

    // --- cuántos puntos de acceso comparten un SSID (mesh) ---

    private fun networkFor(ssid: String, bssid: String) = WifiNetworkInfo(
        ssid = ssid, bssid = bssid, rssiDbm = -50, frequencyMhz = 5240,
        band = WifiBand.GHZ_5, channel = 48, channelWidthLabel = "80 MHz",
        security = WifiSecurity.WPA2, vendor = "", isConnected = false,
    )

    @Test
    fun `apCountForSsid cuenta los BSSID que comparten SSID`() {
        val networks = listOf(
            networkFor("Casa", "aa:aa:aa:aa:aa:01"),
            networkFor("Casa", "aa:aa:aa:aa:aa:02"),
            networkFor("Casa", "aa:aa:aa:aa:aa:03"),
            networkFor("Vecina", "bb:bb:bb:bb:bb:01"),
        )
        assertEquals(3, apCountForSsid(networks, "Casa"))
        assertEquals(1, apCountForSsid(networks, "Vecina"))
    }

    @Test
    fun `apCountForSsid da 0 con SSID vacio`() {
        val networks = listOf(networkFor("", "aa:aa:aa:aa:aa:01"), networkFor("", "aa:aa:aa:aa:aa:02"))
        assertEquals(0, apCountForSsid(networks, ""))
    }

    // --- IP desde CIDR ---

    @Test
    fun `ipFromCidr quita el prefijo`() {
        assertEquals("192.168.0.105", ipFromCidr("192.168.0.105/24"))
    }

    @Test
    fun `ipFromCidr null da null`() {
        assertNull(ipFromCidr(null))
    }

    @Test
    fun `ipFromCidr vacio da null`() {
        assertNull(ipFromCidr(""))
    }
}
