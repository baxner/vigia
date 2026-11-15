package com.baxner.vigia.data.agent

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** [isValidWebhookUrl]: solo http/https con host (ver el contrato § "Webhooks": "Solo `http`/`https`"). */
class WebhookUrlValidationTest {
    @Test
    fun `acepta https con host`() {
        assertTrue(isValidWebhookUrl("https://homeassistant.local:8123/api/webhook/vigia"))
    }

    @Test
    fun `acepta http con host`() {
        assertTrue(isValidWebhookUrl("http://192.168.0.41:8123/webhook"))
    }

    @Test
    fun `recorta espacios antes de validar`() {
        assertTrue(isValidWebhookUrl("  https://ejemplo.com/webhook  "))
    }

    @Test
    fun `rechaza vacio`() {
        assertFalse(isValidWebhookUrl(""))
        assertFalse(isValidWebhookUrl("   "))
    }

    @Test
    fun `rechaza sin esquema`() {
        assertFalse(isValidWebhookUrl("ejemplo.com/webhook"))
    }

    @Test
    fun `rechaza otros esquemas`() {
        assertFalse(isValidWebhookUrl("ftp://ejemplo.com/webhook"))
        assertFalse(isValidWebhookUrl("javascript:alert(1)"))
    }

    @Test
    fun `rechaza esquema valido sin host`() {
        assertFalse(isValidWebhookUrl("https://"))
        assertFalse(isValidWebhookUrl("https:///sin-host"))
    }

    @Test
    fun `rechaza texto que no es una URL`() {
        assertFalse(isValidWebhookUrl("no es una url"))
    }

    @Test
    fun `rechaza hosts loopback`() {
        assertFalse(isValidWebhookUrl("http://localhost:8080/hook"))
        assertFalse(isValidWebhookUrl("https://LOCALHOST/hook"))
        assertFalse(isValidWebhookUrl("http://app.localhost/hook"))
        assertFalse(isValidWebhookUrl("http://127.0.0.1:8080/hook"))
        assertFalse(isValidWebhookUrl("http://127.1.2.3/hook"))
        assertFalse(isValidWebhookUrl("http://[::1]:8080/hook"))
        assertFalse(isValidWebhookUrl("https://[0:0:0:0:0:0:0:1]/hook"))
        assertFalse(isValidWebhookUrl("http://[::ffff:127.0.0.1]/hook"))
    }

    @Test
    fun `rechaza direccion nula y link-local`() {
        assertFalse(isValidWebhookUrl("http://0.0.0.0:8080/hook"))
        assertFalse(isValidWebhookUrl("http://[::]:8080/hook"))
        assertFalse(isValidWebhookUrl("http://169.254.169.254/latest/meta-data"))
        assertFalse(isValidWebhookUrl("http://169.254.0.1/hook"))
        assertFalse(isValidWebhookUrl("http://[fe80::1]/hook"))
    }
}
