package com.baxner.vigia.data.history

import com.baxner.vigia.data.model.Device

/** Clave de `extra` donde el motor pondrá el UUID UPnP del dispositivo (SSDP `USN`/`UDN`). */
internal const val ExtraSsdpUdn = "ssdp:udn"

/** Clave de `extra` con el nombre que el equipo anuncia por mDNS/Bonjour. */
internal const val ExtraMdnsName = "mdns:name"

/**
 * Reconciliación de identidad: decide si un [Device] recién escaneado es un
 * equipo ya conocido (y cuál) o uno nuevo. Pura — sin Room, sin reloj, sin
 * IO — para poder probarla con JUnit normal sobre listas en memoria.
 *
 * Reglas, en orden (la primera que aplica gana). Cada regla es más débil que
 * la anterior, así que cada una se bloquea si CONTRADICE a alguna de las más
 * fuertes que ya se intentaron (mismo identificador, pero con un valor
 * distinto) — no vaya a juntar, por ejemplo, dos Echo Show distintos que
 * casualmente anuncian el mismo nombre, o dos UDN iguales por un error del
 * motor, cuando sus MAC dicen que son equipos distintos:
 *  1. Misma MAC (la del [KnownDevice.mac] "pegajoso": una vez conocida no se
 *     olvida aunque un escaneo puntual no la vuelva a resolver). Las MAC
 *     aleatorias de los teléfonos sirven igual: son estables por red. Nada
 *     más fuerte por delante, no se bloquea por nada.
 *  2. Mismo `extra["ssdp:udn"]` (UUID UPnP) del último [Device] visto —
 *     salvo que la MAC contradiga.
 *  3. Mismo "nombre fuerte" (ver [strongNameOf]) — salvo que la MAC o el UDN
 *     contradigan.
 *  4. Misma última IP — salvo que la MAC, el UDN o el nombre fuerte
 *     contradigan.
 *
 * Cada conocido se usa como máximo una vez por escaneo: si dos escaneados
 * calificarían para el mismo conocido, solo el primero (en el orden de
 * [ScanResult.devices][com.baxner.vigia.data.model.ScanResult]) se lo queda.
 * Sin ninguna regla aplicable, el escaneado es un equipo nuevo (match = null).
 */
object DeviceReconciler {

    /** Un [scanned] del escaneo, emparejado con el conocido al que corresponde (o `null` = nuevo). */
    data class Match(val scanned: Device, val known: KnownDevice?)

    fun reconcile(known: List<KnownDevice>, scanned: List<Device>): List<Match> {
        val available = known.toMutableList()
        return scanned.map { device ->
            val match = findMatch(available, device)
            if (match != null) available.remove(match)
            Match(device, match)
        }
    }

    private fun findMatch(candidates: List<KnownDevice>, device: Device): KnownDevice? {
        if (device.mac.isNotEmpty()) {
            candidates.firstOrNull { it.mac == device.mac }?.let { return it }
        }
        val udn = device.extra[ExtraSsdpUdn]?.takeIf { it.isNotBlank() }
        if (udn != null) {
            candidates.firstOrNull { it.last.extra[ExtraSsdpUdn] == udn && !macConflicts(it, device) }
                ?.let { return it }
        }
        val name = strongNameOf(device)
        if (name != null) {
            candidates.firstOrNull { strongNameOf(it.last) == name && !macConflicts(it, device) && !udnConflicts(it, device) }
                ?.let { return it }
        }
        return candidates.firstOrNull {
            it.lastIp == device.ip && !macConflicts(it, device) && !udnConflicts(it, device) && !nameConflicts(it, device)
        }
    }

    /** [known] y [device] traen cada uno una MAC no vacía, y son distintas. */
    private fun macConflicts(known: KnownDevice, device: Device): Boolean =
        !known.mac.isNullOrEmpty() && device.mac.isNotEmpty() && known.mac != device.mac

    /** [known] y [device] traen cada uno un `ssdp:udn`, y son distintos. */
    private fun udnConflicts(known: KnownDevice, device: Device): Boolean {
        val knownUdn = known.last.extra[ExtraSsdpUdn]?.takeIf { it.isNotBlank() }
        val deviceUdn = device.extra[ExtraSsdpUdn]?.takeIf { it.isNotBlank() }
        return knownUdn != null && deviceUdn != null && knownUdn != deviceUdn
    }

    /** [known] y [device] tienen cada uno un nombre fuerte (ver [strongNameOf]), y son distintos. */
    private fun nameConflicts(known: KnownDevice, device: Device): Boolean {
        val knownName = strongNameOf(known.last)
        val deviceName = strongNameOf(device)
        return knownName != null && deviceName != null && knownName != deviceName
    }

    /** Nombre que Android le pone al hostname cuando aleatoriza por privacidad (p. ej. "Android_1a2b3c4d"). */
    private val RandomAndroidName = Regex("^Android_[A-Za-z0-9]{8}$")

    /** Nombres sin información real que no deben juntar dos equipos distintos. */
    private val JunkName = Regex("^(none(-\\d+)?|localhost)$", RegexOption.IGNORE_CASE)

    /**
     * El nombre "fuerte" de [device] para reconciliar, o `null` si no tiene
     * uno útil: lo que anuncia por mDNS/Bonjour (`extra["mdns:name"]`) o, si
     * no hay, el `hostname` — pero nunca uno que Android aleatoriza por
     * privacidad ni basura genérica ("none", "none-2", "localhost"). Se
     * normaliza a minúsculas: la comparación no distingue mayúsculas.
     */
    fun strongNameOf(device: Device): String? {
        val candidate = device.extra[ExtraMdnsName]?.takeIf { it.isNotBlank() }
            ?: device.hostname.takeIf { it.isNotBlank() }
            ?: return null
        if (RandomAndroidName.matches(candidate) || JunkName.matches(candidate)) return null
        return candidate.lowercase()
    }
}
