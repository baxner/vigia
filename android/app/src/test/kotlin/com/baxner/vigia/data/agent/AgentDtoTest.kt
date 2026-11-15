package com.baxner.vigia.data.agent

import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.DeviceEventType
import com.baxner.vigia.data.model.Device
import java.time.Instant
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Las formas JSON del contrato (ver `docs/agente-api.md`): que [KnownDeviceDto]/
 * [DeviceEventDto] se conviertan bien al dominio de `data/history/`, y sobre
 * todo [DeviceCustomizationPatch] — la pieza que distingue "campo ausente"
 * (sin cambio) de "campo presente con `null`" (borrar), que es la regla más
 * fácil de romper por accidente del PATCH.
 */
class AgentDtoTest {

    @Test
    fun `KnownDeviceDto a dominio parsea fechas e ip`() {
        val dto = KnownDeviceDto(
            id = 12,
            network = "192.168.0.1|192.168.0.0/24",
            mac = "f8:4e:58:af:91:42",
            lastIp = "192.168.0.118",
            firstSeen = "2026-10-06T22:10:00Z",
            lastSeen = "2026-10-07T01:05:00Z",
            online = true,
            onlineSince = "2026-10-06T22:10:00Z",
            last = Device(ip = "192.168.0.118"),
            custom = DeviceCustomizationDto(location = "Sala"),
        )

        val domain = dto.toDomain()

        assertEquals(12L, domain.id)
        assertEquals("192.168.0.1|192.168.0.0/24", domain.network.value)
        assertEquals(Instant.parse("2026-10-06T22:10:00Z"), domain.firstSeen)
        assertEquals(Instant.parse("2026-10-07T01:05:00Z"), domain.lastSeen)
        assertEquals(Instant.parse("2026-10-06T22:10:00Z"), domain.onlineSince)
        assertTrue(domain.online)
        assertEquals("Sala", domain.custom.location)
    }

    @Test
    fun `KnownDeviceDto sin onlineSince queda null`() {
        val dto = KnownDeviceDto(
            id = 1, network = "x", lastIp = "1.2.3.4",
            firstSeen = "2026-01-01T00:00:00Z", lastSeen = "2026-01-01T00:00:00Z",
            online = false, onlineSince = null, last = Device(),
        )
        assertNull(dto.toDomain().onlineSince)
    }

    @Test
    fun `DeviceEventDto a dominio mapea el tipo y la fecha`() {
        val dto = DeviceEventDto(deviceId = 5, type = "IP_CHANGED", at = "2026-10-06T22:10:00Z", detail = "a → b")

        val domain = dto.toDomain()

        assertEquals(5L, domain.deviceId)
        assertEquals(DeviceEventType.IP_CHANGED, domain.type)
        assertEquals("a → b", domain.detail)
    }

    @Test
    fun `replaceAll manda los 5 campos siempre presentes`() {
        val json = DeviceCustomizationPatch.replaceAll(
            DeviceCustomization(name = "Foco", typeId = null, notes = null, location = "Sala", important = true),
        ).toJsonObject()

        assertEquals(5, json.size)
        assertEquals("Foco", json["name"]?.jsonPrimitive?.content)
        assertEquals(JsonNull, json["typeId"])
        assertEquals(JsonNull, json["notes"])
        assertEquals("Sala", json["location"]?.jsonPrimitive?.content)
        assertEquals(true, json["important"]?.jsonPrimitive?.content?.toBoolean())
    }

    @Test
    fun `onlySet omite por completo los campos sin valor, no los manda como null`() {
        val json = DeviceCustomizationPatch.onlySet(
            DeviceCustomization(name = "Foco", typeId = null, notes = null, location = null, important = false),
        ).toJsonObject()

        assertEquals(setOf("name"), json.keys)
        assertFalse(json.containsKey("typeId"))
        assertFalse(json.containsKey("important"))
    }

    @Test
    fun `onlySet incluye important solo cuando es true`() {
        val json = DeviceCustomizationPatch.onlySet(DeviceCustomization(important = true)).toJsonObject()
        assertEquals(setOf("important"), json.keys)
        assertEquals(true, json["important"]?.jsonPrimitive?.content?.toBoolean())
    }

    @Test
    fun `un PatchField Absent no aparece en el JSON mientras que Present null si aparece`() {
        val absent = DeviceCustomizationPatch(name = PatchField.Absent).toJsonObject()
        val presentNull = DeviceCustomizationPatch(name = PatchField.Present(null)).toJsonObject()

        assertFalse(absent.containsKey("name"))
        assertTrue(presentNull.containsKey("name"))
        assertEquals(JsonNull, presentNull["name"])
    }
}
