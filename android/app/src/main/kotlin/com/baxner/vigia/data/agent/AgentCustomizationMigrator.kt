package com.baxner.vigia.data.agent

import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.DeviceReconciler
import com.baxner.vigia.data.history.KnownDevice

/** Tiene algo que valga la pena subir (ver [AgentCustomizationMigrator.plan]). */
fun DeviceCustomization.hasCustomization(): Boolean =
    name != null || typeId != null || notes != null || location != null || important

/**
 * Al vincular por primera vez: qué le toca mandar al agente de las
 * personalizaciones que el usuario ya hizo en esta red con el historial
 * local (ver el encargo, punto 4). Pura — listas en memoria, sin red — para
 * poder probarla con JUnit plano; quien la llama ([com.baxner.vigia.data.agent.AgentModeController])
 * sí hace las llamadas HTTP con el resultado.
 *
 * Empareja cada [local] personalizado con el [remote] correspondiente por
 * MAC; si a alguno de los dos le falta la MAC, por IP + "nombre fuerte" del
 * motor ([DeviceReconciler.strongNameOf] — el mismo criterio que usa la
 * reconciliación del historial, no una IP sola que podría ser de otro
 * equipo). Un mismo [remote] no se usa dos veces. Nunca borra nada: solo
 * manda los campos que [local] trae puestos ([DeviceCustomizationPatch.onlySet]),
 * así que no hay forma de que esto limpie una personalización que el agente
 * ya tuviera por su cuenta.
 */
object AgentCustomizationMigrator {
    fun plan(local: List<KnownDevice>, remote: List<KnownDevice>): List<Pair<Long, DeviceCustomizationPatch>> {
        val customized = local.filter { it.custom.hasCustomization() }
        if (customized.isEmpty()) return emptyList()

        val usedRemoteIds = mutableSetOf<Long>()
        return customized.mapNotNull { localDevice ->
            val match = remote.firstOrNull { it.id !in usedRemoteIds && matches(localDevice, it) } ?: return@mapNotNull null
            usedRemoteIds += match.id
            match.id to DeviceCustomizationPatch.onlySet(localDevice.custom)
        }
    }

    private fun matches(local: KnownDevice, remote: KnownDevice): Boolean {
        val localMac = local.mac
        val remoteMac = remote.mac
        if (!localMac.isNullOrBlank() && !remoteMac.isNullOrBlank()) {
            return localMac.equals(remoteMac, ignoreCase = true)
        }
        // Sin MAC de un lado (p. ej. el agente todavía no la resolvió): solo
        // misma IP no basta (pudo cambiar de dueño); exige también el mismo
        // nombre fuerte, y que exista (sin nombre, no se arriesga el empate).
        val localName = DeviceReconciler.strongNameOf(local.last)
        val remoteName = DeviceReconciler.strongNameOf(remote.last)
        return local.lastIp == remote.lastIp && localName != null && localName == remoteName
    }
}
