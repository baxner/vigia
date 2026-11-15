package com.baxner.vigia.data.model

/**
 * Tipos de dispositivo que reconoce Vigía. Es una capa sobre el `identity.type`
 * de texto libre que manda el motor: así la UI (icono, filtro, etiqueta) no
 * depende de comparar strings sueltos por todos lados, y cualquier valor que
 * el motor agregue a futuro cae en [UNKNOWN] en vez de romper la app.
 */
enum class DeviceType(val raw: String) {
    PHONE("phone"),
    COMPUTER("computer"),
    TV("tv"),
    ROUTER("router"),
    PRINTER("printer"),
    CAMERA("camera"),
    SPEAKER("speaker"),
    CONSOLE("console"),
    NAS("nas"),
    IOT("iot"),
    WEARABLE("wearable"),
    TABLET("tablet"),
    UNKNOWN("unknown");

    companion object {
        private val byRaw = entries.associateBy { it.raw }

        fun fromRaw(raw: String): DeviceType = byRaw[raw.lowercase()] ?: UNKNOWN
    }
}

/** Nombres que no dicen nada (el motor ya filtra la mayoría; por si acaso). */
private val junkHostname = Regex("(?i)^(none|localhost|unknown|android)(-\\d+)?$")

/**
 * El nombre humano del dispositivo, o null si no tiene ninguno: nombre que
 * anuncia por Bonjour ("Mac-Estudio") > hostname > "marca modelo" ("Router-Lab
 * AX3000") > marca ("Hikvision") > fabricante crudo del OUI. Un hostname que es solo la marca ("Samsung")
 * cede ante "Samsung TV-Sala-75".
 */
fun Device.nameOrNull(): String? {
    val instance = extra["mdns:name"].orEmpty().trim()
    val host = hostname.trim().takeUnless { it.isEmpty() || junkHostname.matches(it) }
    val brand = identity.brand.trim()
    val model = identity.model.trim()
    val brandModel = listOf(brand, model).filter { it.isNotEmpty() }.distinct().joinToString(" ")
    return when {
        instance.isNotEmpty() -> instance
        host != null && !(host.equals(brand, ignoreCase = true) && model.isNotEmpty()) -> host
        model.isNotEmpty() -> brandModel
        brand.isNotEmpty() -> brand // marca limpia del motor ("Hikvision"), no la razón social
        vendor.isNotBlank() -> vendor
        else -> null
    }
}

/** [nameOrNull] o, si no hay ninguno, la IP. */
fun Device.displayName(): String = nameOrNull() ?: ip

/** El propio teléfono que escanea (lo marca el motor). */
val Device.isSelf: Boolean get() = extra["self"] == "true"
