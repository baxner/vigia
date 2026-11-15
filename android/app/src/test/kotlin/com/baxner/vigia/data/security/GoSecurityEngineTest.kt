package com.baxner.vigia.data.security

import com.baxner.vigia.data.agent.SecurityDevice
import com.baxner.vigia.data.model.Device
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * El JSON que recibe el motor en Go: Go lee un campo ausente como su valor
 * cero, así que los valores por defecto de Kotlin tienen que ir escritos
 * (ver [encodeEngineInput]).
 */
class GoSecurityEngineTest {

    @Test
    fun `los valores por defecto van escritos para el motor`() {
        val input = SecurityInput(
            devices = listOf(SecurityDevice(id = 7, name = "PC", device = Device(ip = "192.168.0.55"))),
            wifi = WifiInfo(ssid = "Casa"), // security por defecto: "open"
        )

        val json = Json.parseToJsonElement(encodeEngineInput(input)).jsonObject

        val device = json["devices"]!!.jsonArray[0].jsonObject
        assertTrue("online tiene que ir aunque sea el valor por defecto", device["online"]!!.jsonPrimitive.boolean)
        assertEquals("open", json["wifi"]!!.jsonObject["security"]!!.jsonPrimitive.content)
        assertEquals("192.168.0.55", device["device"]!!.jsonObject["ip"]!!.jsonPrimitive.content)
    }
}
