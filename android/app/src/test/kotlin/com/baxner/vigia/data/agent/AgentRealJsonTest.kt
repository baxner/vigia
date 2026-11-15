package com.baxner.vigia.data.agent

import com.baxner.vigia.data.repository.VigiaJson
import kotlinx.serialization.builtins.ListSerializer
import org.junit.Assert.assertEquals
import org.junit.Test

/** La respuesta REAL del agente (/networks/{key}/devices, anonimizada) debe parsear entera. */
class AgentRealJsonTest {
    @Test
    fun `la lista real del agente se parsea y se convierte`() {
        val json = javaClass.classLoader!!.getResource("agent-devices-real.json")!!.readText()
        val dtos = VigiaJson.decodeFromString(ListSerializer(KnownDeviceDto.serializer()), json)
        assertEquals(22, dtos.size)
        val domain = dtos.map { it.toDomain() }
        assertEquals(22, domain.size)
    }
}
