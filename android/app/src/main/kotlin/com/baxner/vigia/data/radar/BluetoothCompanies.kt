package com.baxner.vigia.data.radar

import android.content.Context
import java.io.BufferedReader
import java.io.InputStreamReader

/*
 * Fase 6, "Buscar señal" (ver docs/plan.md): marca de un equipo Bluetooth LE
 * por el identificador de fabricante de su anuncio (lista oficial del
 * Bluetooth SIG). La lista se bajó una vez con curl desde
 * https://bitbucket.org/bluetooth-SIG/public/raw/main/assigned_numbers/company_identifiers/company_identifiers.yaml
 * (4045 fabricantes, verificada sin duplicados ni filas rotas) y se convirtió
 * a app/src/main/assets/bt_companies.tsv ("id decimal<TAB>nombre completo"),
 * que queda versionado en el repo: no se vuelve a bajar en cada build.
 */

/**
 * Nombres de fabricante Bluetooth por su identificador, cargados de
 * [AssetPath]. Si el asset falta o está corrupto, [load] no falla: devuelve
 * una lista vacía (brandFor/fullNameFor dan null para todo) para que un
 * problema de datos no tumbe el escaneo BLE.
 */
class BluetoothCompanies private constructor(private val namesById: Map<Int, String>) {

    /** Cuántos fabricantes conoce (0 si el asset no se pudo leer). */
    val knownCompanyCount: Int get() = namesById.size

    /** Nombre completo tal cual lo publica el SIG ("Apple, Inc."); null si no se conoce [companyId]. */
    fun fullNameFor(companyId: Int): String? = namesById[companyId]

    /** Nombre corto para mostrar en el radar ("Apple"); null si no se conoce [companyId]. */
    fun brandFor(companyId: Int): String? = namesById[companyId]?.let(::shortName)

    companion object {
        private const val AssetPath = "bt_companies.tsv"

        fun load(context: Context): BluetoothCompanies {
            val names = runCatching {
                context.applicationContext.assets.open(AssetPath).use { input ->
                    parseTsv(BufferedReader(InputStreamReader(input, Charsets.UTF_8)).readLines())
                }
            }.getOrElse { emptyMap() }
            return BluetoothCompanies(names)
        }

        /**
         * "id decimal<TAB>nombre" por línea, una entrada por fabricante. Aparte
         * para poder probarla con JUnit plano, sin `Context` ni `AssetManager`.
         * Ignora líneas en blanco o mal formadas en vez de romper la carga
         * entera por una fila rota.
         */
        fun parseTsv(lines: List<String>): Map<Int, String> {
            val names = LinkedHashMap<Int, String>()
            for (line in lines) {
                if (line.isBlank()) continue
                val tab = line.indexOf('\t')
                if (tab <= 0) continue
                val id = line.substring(0, tab).toIntOrNull() ?: continue
                val name = line.substring(tab + 1).trim()
                if (name.isNotEmpty()) names[id] = name
            }
            return names
        }

        /**
         * Sufijos legales comunes que se recortan al final del nombre
         * ("Apple, Inc." -> "Apple", "Samsung Electronics Co. Ltd." ->
         * "Samsung Electronics"). Mejor esfuerzo: con más de 4000 fabricantes
         * en la lista del SIG (muchos con razones sociales raras: "s.r.o.",
         * "d.o.o.", nombres chinos transliterados...) no hay forma de
         * acertar siempre sin una lista hecha a mano por fabricante. La
         * comparación es sin distinguir mayúsculas.
         */
        private val LegalSuffixes = listOf(
            "Incorporated", "Inc", "LLC", "L.L.C",
            "Limited", "Ltd",
            "Corporation", "Corp",
            "Company", "Co",
            "GmbH", "AG", "KG",
            "S.A.U", "S.A", "S.L", "S.p.A", "S.r.l",
            "B.V", "N.V",
            "A/S", "ApS",
            "Oy", "AB",
            "SAS", "SARL",
            "PLC",
            "Pty", "Pte",
            "d.o.o", "s.r.o", "K.K", "JSC", "UAB", "Lda",
        )

        // Precompiladas una sola vez: se evalúan por cada equipo BLE visto.
        private val SuffixPatterns = LegalSuffixes.map { suffix ->
            Regex("""(?i)(,\s*|\s+)${Regex.escape(suffix)}\.?\s*$""")
        }

        /**
         * Quita hasta 3 sufijos legales encadenados (p. ej. "Co." y luego
         * "Ltd." en "Co., Ltd."). Si ninguno aplica, devuelve el nombre tal
         * cual: no es un error, solo un nombre que esta heurística no sabe
         * recortar.
         */
        fun shortName(fullName: String): String {
            var name = fullName.trim()
            if (name.isEmpty()) return name
            var attempts = 0
            while (attempts < 3) {
                val next = stripOneSuffix(name) ?: break
                name = next
                attempts++
            }
            return name
        }

        private fun stripOneSuffix(name: String): String? {
            for (pattern in SuffixPatterns) {
                if (pattern.containsMatchIn(name)) {
                    val stripped = name.replace(pattern, "").trimEnd(' ', ',', '.')
                    if (stripped.isNotBlank()) return stripped
                }
            }
            return null
        }
    }
}
