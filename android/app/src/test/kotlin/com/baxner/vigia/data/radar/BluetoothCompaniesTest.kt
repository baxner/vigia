package com.baxner.vigia.data.radar

import org.junit.Assert.assertEquals
import org.junit.Test

/** Carga del TSV y nombre corto de fabricante Bluetooth (ver BluetoothCompanies.kt). */
class BluetoothCompaniesTest {

    // ---- Carga del TSV ----

    @Test
    fun `lee id y nombre separados por tab`() {
        val names = BluetoothCompanies.parseTsv(listOf("76\tApple, Inc.", "117\tSamsung Electronics Co. Ltd."))
        assertEquals("Apple, Inc.", names[76])
        assertEquals("Samsung Electronics Co. Ltd.", names[117])
        assertEquals(2, names.size)
    }

    @Test
    fun `ignora lineas en blanco y filas rotas sin romper la carga`() {
        val names = BluetoothCompanies.parseTsv(
            listOf("", "no-tiene-tab", "\tsin id", "224\t", "369\tAmazon.com Services LLC", "   "),
        )
        assertEquals(mapOf(369 to "Amazon.com Services LLC"), names)
    }

    @Test
    fun `una fila con id invalido no tumba el resto`() {
        val names = BluetoothCompanies.parseTsv(listOf("seis\tMicrosoft", "6\tMicrosoft"))
        assertEquals(mapOf(6 to "Microsoft"), names)
    }

    // ---- IDs de referencia (verificados contra la lista real del SIG bajada) ----

    @Test
    fun `los ids de marcas comunes resuelven al fabricante esperado`() {
        val names = mapOf(
            76 to "Apple, Inc.",
            117 to "Samsung Electronics Co. Ltd.",
            224 to "Google",
            369 to "Amazon.com Services LLC",
            6 to "Microsoft",
            911 to "Xiaomi Inc.",
        )
        for ((id, fullName) in names) {
            assertEquals(fullName, BluetoothCompanies.parseTsv(listOf("$id\t$fullName"))[id])
        }
    }

    // ---- Nombre corto ----

    @Test
    fun `quita el sufijo legal mas comun`() {
        assertEquals("Apple", BluetoothCompanies.shortName("Apple, Inc."))
        assertEquals("Xiaomi", BluetoothCompanies.shortName("Xiaomi Inc."))
        assertEquals("Intel", BluetoothCompanies.shortName("Intel Corp."))
        assertEquals("Microsoft", BluetoothCompanies.shortName("Microsoft"))
    }

    @Test
    fun `quita sufijos encadenados`() {
        assertEquals("Samsung Electronics", BluetoothCompanies.shortName("Samsung Electronics Co. Ltd."))
    }

    @Test
    fun `sin sufijo conocido devuelve el nombre tal cual`() {
        assertEquals("Google", BluetoothCompanies.shortName("Google"))
        assertEquals("FUSE BLUEJAY LIMITED".let { BluetoothCompanies.shortName(it) }, BluetoothCompanies.shortName("FUSE BLUEJAY LIMITED"))
    }

    @Test
    fun `no recorta un sufijo pegado sin espacio (no es una palabra aparte)`() {
        // "Jamco" termina en "...amco": no es la palabra "Co" sola, no debe tocarse.
        assertEquals("Jamco", BluetoothCompanies.shortName("Jamco"))
    }

    @Test
    fun `nombre vacio no rompe nada`() {
        assertEquals("", BluetoothCompanies.shortName(""))
        assertEquals("", BluetoothCompanies.shortName("   "))
    }
}
