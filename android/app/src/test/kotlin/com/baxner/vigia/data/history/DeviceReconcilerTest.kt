package com.baxner.vigia.data.history

import com.baxner.vigia.data.model.Device
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * La reconciliación es pura (sin Room): se prueba armando [KnownDevice] y
 * [Device] a mano, sin tocar disco ni reloj real.
 */
class DeviceReconcilerTest {

    private val network = NetworkKey.of("Casa", "192.168.0.1", "192.168.0.0/24")
    private val t0: Instant = Instant.parse("2026-01-01T10:00:00Z")

    private fun device(
        ip: String,
        mac: String = "",
        hostname: String = "",
        extra: Map<String, String> = emptyMap(),
    ) = Device(ip = ip, mac = mac, hostname = hostname, extra = extra)

    private fun known(id: Long, last: Device, mac: String? = last.mac.ifEmpty { null }) = KnownDevice(
        id = id,
        network = network,
        mac = mac,
        lastIp = last.ip,
        firstSeen = t0,
        lastSeen = t0,
        online = true,
        onlineSince = t0,
        last = last,
    )

    @Test
    fun `misma MAC junta aunque cambie la IP`() {
        val k = known(1, device(ip = "192.168.0.10", mac = "aa:bb:cc:dd:ee:01"))
        val escaneado = device(ip = "192.168.0.99", mac = "aa:bb:cc:dd:ee:01")

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertEquals(1L, result.single().known?.id)
    }

    @Test
    fun `mismo ssdp udn junta aunque no haya MAC`() {
        val k = known(1, device(ip = "192.168.0.10", extra = mapOf(ExtraSsdpUdn to "uuid-5678")))
        val escaneado = device(ip = "192.168.0.20", extra = mapOf(ExtraSsdpUdn to "uuid-5678"))

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertEquals(1L, result.single().known?.id)
    }

    @Test
    fun `mismo ssdp udn pero MAC distinta no se junta`() {
        // Dos equipos reales con MAC propia; que el UDN coincida (p. ej. un bug del motor) no basta.
        val k = known(1, device(ip = "192.168.0.10", mac = "aa:bb:cc:dd:ee:01", extra = mapOf(ExtraSsdpUdn to "uuid-dup")))
        val escaneado = device(ip = "192.168.0.20", mac = "11:22:33:44:55:66", extra = mapOf(ExtraSsdpUdn to "uuid-dup"))

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertNull("la MAC contradice: no puede ser el mismo equipo aunque el UDN coincida", result.single().known)
    }

    @Test
    fun `mismo nombre fuerte pero MAC distinta no se junta (dos Echo Show)`() {
        val k = known(1, device(ip = "192.168.0.52", mac = "aa:bb:cc:dd:ee:01", extra = mapOf(ExtraMdnsName to "Echo Show")))
        // Otro Echo Show real con su propia MAC: el nombre coincide pero no es el mismo equipo.
        val escaneado = device(ip = "192.168.0.53", mac = "11:22:33:44:55:66", extra = mapOf(ExtraMdnsName to "Echo Show"))

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertNull("la MAC contradice: no puede ser el mismo Echo Show", result.single().known)
    }

    @Test
    fun `mismo nombre fuerte pero UDN distinto no se junta`() {
        val k = known(1, device(ip = "192.168.0.52", extra = mapOf(ExtraMdnsName to "Echo Show", ExtraSsdpUdn to "uuid-1")))
        val escaneado = device(ip = "192.168.0.53", extra = mapOf(ExtraMdnsName to "Echo Show", ExtraSsdpUdn to "uuid-2"))

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertNull("el UDN contradice aunque el nombre coincida", result.single().known)
    }

    @Test
    fun `Mac-Estudio se reconoce por nombre fuerte aunque cambie de IP`() {
        val previo = device(ip = "192.168.0.48", hostname = "Mac-Estudio", extra = mapOf(ExtraMdnsName to "Mac-Estudio"))
        val k = known(1, previo, mac = "02:00:00:12:ab:34")
        // Se fue a otra IP y esta vez ni siquiera trae la MAC resuelta (p. ej. ARP tardó).
        val escaneado = device(ip = "192.168.0.50", extra = mapOf(ExtraMdnsName to "Mac-Estudio"))

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertEquals(1L, result.single().known?.id)
    }

    @Test
    fun `un Android con hostname aleatorio no se junta por nombre`() {
        val k = known(1, device(ip = "192.168.0.60", hostname = "Android_1a2b3c4d"))
        // Mismo hostname "aleatorio" pero en otra IP y sin MAC: si el nombre contara, se juntarían.
        val escaneado = device(ip = "192.168.0.61", hostname = "Android_1a2b3c4d")

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertNull("un hostname aleatorizado por Android no debe reconciliar por nombre", result.single().known)
    }

    @Test
    fun `basura como none o localhost no cuenta como nombre fuerte`() {
        assertNull(DeviceReconciler.strongNameOf(device(ip = "1.2.3.4", hostname = "none")))
        assertNull(DeviceReconciler.strongNameOf(device(ip = "1.2.3.4", hostname = "none-2")))
        assertNull(DeviceReconciler.strongNameOf(device(ip = "1.2.3.4", hostname = "localhost")))
        assertNull(DeviceReconciler.strongNameOf(device(ip = "1.2.3.4", hostname = "LocalHost")))
    }

    @Test
    fun `dos equipos sin nombre ni MAC se distinguen por IP aunque se intercambien`() {
        val k1 = known(1, device(ip = "192.168.0.10"))
        val k2 = known(2, device(ip = "192.168.0.11"))
        // El escaneo ve un equipo en cada IP de siempre: sin MAC ni nombre, la única pista es
        // la IP, así que cada escaneado reconcilia con el conocido de esa IP (si físicamente se
        // intercambiaron las cajas, el sistema no tiene forma de saberlo con lo que tiene).
        val enDiez = device(ip = "192.168.0.10")
        val enOnce = device(ip = "192.168.0.11")

        val result = DeviceReconciler.reconcile(listOf(k1, k2), listOf(enDiez, enOnce))

        assertEquals(1L, result[0].known?.id)
        assertEquals(2L, result[1].known?.id)
    }

    @Test
    fun `el PC con MAC por NetBIOS se reconoce por esa MAC aunque cambie de IP`() {
        val previo = device(ip = "192.168.0.4", mac = "02:00:00:44:51:0a", hostname = "PC-Escritorio")
        val k = known(1, previo)
        val escaneado = device(ip = "192.168.0.5", mac = "02:00:00:44:51:0a", hostname = "PC-Escritorio")

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertEquals(1L, result.single().known?.id)
    }

    @Test
    fun `una MAC distinta contradice y bloquea el match por IP`() {
        val k = known(1, device(ip = "192.168.0.10", mac = "aa:bb:cc:dd:ee:01"))
        // Misma IP, pero otra MAC: no puede ser el mismo equipo.
        val escaneado = device(ip = "192.168.0.10", mac = "ff:ee:dd:cc:bb:02")

        val result = DeviceReconciler.reconcile(listOf(k), listOf(escaneado))

        assertNull(result.single().known)
    }

    @Test
    fun `sin ninguna pista es un equipo nuevo`() {
        val result = DeviceReconciler.reconcile(emptyList(), listOf(device(ip = "192.168.0.200")))
        assertNull(result.single().known)
    }

    @Test
    fun `cada conocido se usa una sola vez por escaneo`() {
        val k = known(1, device(ip = "192.168.0.10"))
        // Dos escaneados reclamando la misma IP (caso límite): solo el primero se queda el conocido.
        val result = DeviceReconciler.reconcile(
            listOf(k),
            listOf(device(ip = "192.168.0.10"), device(ip = "192.168.0.10")),
        )

        assertEquals(1L, result[0].known?.id)
        assertNull(result[1].known)
    }
}
