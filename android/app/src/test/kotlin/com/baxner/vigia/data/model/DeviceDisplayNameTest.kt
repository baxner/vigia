package com.baxner.vigia.data.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** hostname > marca+modelo > fabricante > marca > IP, en ese orden. */
class DeviceDisplayNameTest {

    @Test
    fun `con hostname, gana el hostname aunque haya modelo y fabricante`() {
        val device = Device(
            ip = "192.168.0.50",
            hostname = "mi-pc.local",
            vendor = "Dell",
            identity = Identity(model = "XPS 13"),
        )
        assertEquals("mi-pc.local", device.displayName())
    }

    @Test
    fun `sin hostname, gana el modelo`() {
        val device = Device(ip = "192.168.0.50", vendor = "Dell", identity = Identity(model = "XPS 13"))
        assertEquals("XPS 13", device.displayName())
    }

    @Test
    fun `sin hostname, marca y modelo juntos`() {
        val device = Device(ip = "192.168.0.4", vendor = "TP-Link Corporation", identity = Identity(brand = "Router-Lab", model = "AX3000"))
        assertEquals("Router-Lab AX3000", device.displayName())
    }

    @Test
    fun `marca repetida en el modelo no se duplica`() {
        val device = Device(ip = "192.168.0.9", identity = Identity(brand = "Samsung", model = "Samsung"))
        assertEquals("Samsung", device.displayName())
    }

    @Test
    fun `sin fabricante por MAC (aleatoria), usa la marca deducida`() {
        val device = Device(ip = "192.168.0.53", identity = Identity(brand = "Samsung"))
        assertEquals("Samsung", device.displayName())
    }

    @Test
    fun `el nombre que anuncia por Bonjour gana al hostname`() {
        val device = Device(ip = "192.168.0.48", hostname = "mac-estudio", extra = mapOf("mdns:name" to "Mac-Estudio"))
        assertEquals("Mac-Estudio", device.displayName())
    }

    @Test
    fun `un hostname basura no es un nombre`() {
        val device = Device(ip = "192.168.0.62", hostname = "none-2", identity = Identity(type = "speaker"))
        assertNull(device.nameOrNull())
        assertEquals("192.168.0.62", device.displayName())
    }

    @Test
    fun `un hostname que es solo la marca cede ante marca y modelo`() {
        val device = Device(ip = "192.168.0.118", hostname = "Samsung", identity = Identity(brand = "Samsung", model = "TV-Sala-75"))
        assertEquals("Samsung TV-Sala-75", device.displayName())
    }

    @Test
    fun `isSelf solo con la marca del motor`() {
        assertEquals(true, Device(extra = mapOf("self" to "true")).isSelf)
        assertEquals(false, Device().isSelf)
    }

    @Test
    fun `la marca limpia del motor gana a la razon social del OUI`() {
        val device = Device(
            ip = "192.168.0.41",
            vendor = "Hangzhou Hikvision Digital Technology Co.,Ltd.",
            identity = Identity(type = "camera", brand = "Hikvision"),
        )
        assertEquals("Hikvision", device.displayName())
    }

    @Test
    fun `sin hostname ni modelo, gana el fabricante`() {
        val device = Device(ip = "192.168.0.50", vendor = "Dell")
        assertEquals("Dell", device.displayName())
    }

    @Test
    fun `sin nada de lo anterior, usa la IP`() {
        val device = Device(ip = "192.168.0.50")
        assertEquals("192.168.0.50", device.displayName())
    }

    @Test
    fun `hostname en blanco no cuenta como hostname`() {
        val device = Device(ip = "192.168.0.50", hostname = "   ", vendor = "Dell")
        assertEquals("Dell", device.displayName())
    }

    @Test
    fun `modelo en blanco no cuenta como modelo`() {
        val device = Device(ip = "192.168.0.50", vendor = "Dell", identity = Identity(model = "  "))
        assertEquals("Dell", device.displayName())
    }
}

class DeviceTypeTest {

    @Test
    fun `fromRaw reconoce los tipos documentados`() {
        assertEquals(DeviceType.ROUTER, DeviceType.fromRaw("router"))
        assertEquals(DeviceType.CAMERA, DeviceType.fromRaw("camera"))
        assertEquals(DeviceType.NAS, DeviceType.fromRaw("nas"))
    }

    @Test
    fun `fromRaw no distingue mayusculas`() {
        assertEquals(DeviceType.PHONE, DeviceType.fromRaw("PHONE"))
    }

    @Test
    fun `fromRaw cae en UNKNOWN si el motor manda un tipo nuevo`() {
        assertEquals(DeviceType.UNKNOWN, DeviceType.fromRaw("algo-que-el-motor-agregue-despues"))
    }
}
