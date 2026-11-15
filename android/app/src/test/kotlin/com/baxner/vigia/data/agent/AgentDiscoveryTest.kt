package com.baxner.vigia.data.agent

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * [decodeNsdTxtRecords]: pura, sin `NsdManager` real (ver el contrato,
 * descubrimiento `_vigia._tcp`, TXT `v`/`id`/`name`).
 */
class AgentDiscoveryTest {
    @Test
    fun `decodifica los bytes UTF-8 de cada registro TXT`() {
        val attributes = mapOf(
            "v" to "1".toByteArray(Charsets.UTF_8),
            "id" to "agent-1".toByteArray(Charsets.UTF_8),
            "name" to "Mac-Estudio".toByteArray(Charsets.UTF_8),
        )

        val decoded = decodeNsdTxtRecords(attributes)

        assertEquals("1", decoded["v"])
        assertEquals("agent-1", decoded["id"])
        assertEquals("Mac-Estudio", decoded["name"])
    }

    @Test
    fun `decodifica el atributo TXT cert y normaliza la huella SHA-256`() {
        val attributes = mapOf(
            "id" to "agent-1".toByteArray(Charsets.UTF_8),
            "name" to "Mac-Estudio".toByteArray(Charsets.UTF_8),
            "cert" to "AA:BB:CC:00:11:22".toByteArray(Charsets.UTF_8),
        )

        val decoded = decodeNsdTxtRecords(attributes)

        assertEquals("aabbcc001122", AgentTls.normalizeFingerprint(decoded["cert"]))
    }

    @Test
    fun `un valor null se omite en vez de reventar`() {
        val attributes = mapOf("id" to "agent-1".toByteArray(Charsets.UTF_8), "name" to null)

        val decoded = decodeNsdTxtRecords(attributes)

        assertEquals("agent-1", decoded["id"])
        assertTrue(!decoded.containsKey("name"))
    }

    @Test
    fun `sin registros devuelve mapa vacio`() {
        assertTrue(decodeNsdTxtRecords(emptyMap()).isEmpty())
    }
}
