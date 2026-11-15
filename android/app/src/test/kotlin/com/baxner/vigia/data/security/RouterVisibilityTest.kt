package com.baxner.vigia.data.security

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * "Tu router visto desde aquí" (ver el encargo, punto 3, y el contrato §
 * "Seguridad" → "Anillos"): pura, sin red ni Android. El caso real: el
 * router principal sale como 192.168.100.2 (anillo exterior, detrás de la ONT); el
 * teléfono, conectado a la Wi-Fi de la ONT, tiene una IP como
 * 192.168.100.50/24 — misma subred, debe detectarse.
 */
class RouterVisibilityTest {
    @Test
    fun `el telefono en la Wi-Fi de la ONT ve al router como exterior`() {
        assertTrue(RouterVisibility.routerSeenFromHere(phoneCidr = "192.168.100.50/24", routerExternalIp = "192.168.100.2"))
    }

    @Test
    fun `el telefono en la red de casa (anillo interior) no lo ve como exterior`() {
        assertFalse(RouterVisibility.routerSeenFromHere(phoneCidr = "192.168.0.23/24", routerExternalIp = "192.168.100.2"))
    }

    @Test
    fun `sin phoneCidr nunca aplica`() {
        assertFalse(RouterVisibility.routerSeenFromHere(phoneCidr = null, routerExternalIp = "192.168.100.2"))
        assertFalse(RouterVisibility.routerSeenFromHere(phoneCidr = "", routerExternalIp = "192.168.100.2"))
    }

    @Test
    fun `sin un RouterInfo guardado (externalIp null) nunca aplica`() {
        assertFalse(RouterVisibility.routerSeenFromHere(phoneCidr = "192.168.100.50/24", routerExternalIp = null))
        assertFalse(RouterVisibility.routerSeenFromHere(phoneCidr = "192.168.100.50/24", routerExternalIp = ""))
    }

    @Test
    fun `respeta el prefijo de la subred del telefono, no asume siempre 24`() {
        // /23 cubre 192.168.100.0 - 192.168.101.255: .2 y 101.50 caen en la misma subred.
        assertTrue(RouterVisibility.routerSeenFromHere(phoneCidr = "192.168.101.50/23", routerExternalIp = "192.168.100.2"))
        // Con /24 esos mismos dos ya no coinciden (subredes distintas).
        assertFalse(RouterVisibility.routerSeenFromHere(phoneCidr = "192.168.101.50/24", routerExternalIp = "192.168.100.2"))
    }

    @Test
    fun `una IP de salida publica (reach internet) no esta en la subred local del telefono`() {
        assertFalse(RouterVisibility.routerSeenFromHere(phoneCidr = "192.168.0.23/24", routerExternalIp = "201.33.44.5"))
    }
}
