package com.baxner.vigia.data.security

import android.content.Context
import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.repository.VigiaJson
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString

/**
 * Lo que la app necesita recordar entre sesiones para Seguridad (ver el
 * encargo, punto 2): el último [RouterInfo] visto, para "Tu router visto
 * desde aquí" (ver [RouterVisibility]) incluso antes de que este proceso
 * haya vuelto a pedir un informe. Mismo patrón que
 * [com.baxner.vigia.data.agent.AlertsStore]/[com.baxner.vigia.data.agent.SharedPrefsAlertsStore]:
 * interfaz aparte para poder probar el resto de la lógica con un fake en
 * memoria. No va cifrado: nada acá es secreto (ni token, ni credenciales).
 */
interface SecurityStore {
    /** `null` si todavía no se guardó ningún informe con router (ni del agente ni del motor local). */
    fun lastRouterInfo(): RouterInfo?
    fun setLastRouterInfo(info: RouterInfo)

    /**
     * "Ya lo sé, es a propósito" sin agente (ver el contrato § "Hallazgos
     * reconocidos v1.3": "la app sin agente los guarda en el teléfono, por
     * red"), para la red [network] (ver [localNetworkKeyFor]). Vacío si no
     * hay ninguno guardado para esa red.
     */
    fun acknowledgedFindings(network: NetworkKey): List<AcknowledgedFinding>
    fun setAcknowledgedFindings(network: NetworkKey, items: List<AcknowledgedFinding>)
}

class SharedPrefsSecurityStore(context: Context) : SecurityStore {
    private val prefs = context.applicationContext.getSharedPreferences(PrefsFileName, Context.MODE_PRIVATE)

    override fun lastRouterInfo(): RouterInfo? =
        prefs.getString(KeyRouterInfo, null)
            ?.let { runCatching { VigiaJson.decodeFromString<RouterInfo>(it) }.getOrNull() }

    override fun setLastRouterInfo(info: RouterInfo) {
        prefs.edit().putString(KeyRouterInfo, VigiaJson.encodeToString(info)).apply()
    }

    override fun acknowledgedFindings(network: NetworkKey): List<AcknowledgedFinding> =
        allAcknowledged()[network.value].orEmpty()

    override fun setAcknowledgedFindings(network: NetworkKey, items: List<AcknowledgedFinding>) {
        val updated = allAcknowledged() + (network.value to items)
        prefs.edit().putString(KeyAcknowledged, VigiaJson.encodeToString(updated)).apply()
    }

    private fun allAcknowledged(): Map<String, List<AcknowledgedFinding>> =
        prefs.getString(KeyAcknowledged, null)
            ?.let { runCatching { VigiaJson.decodeFromString<Map<String, List<AcknowledgedFinding>>>(it) }.getOrNull() }
            .orEmpty()

    private companion object {
        const val PrefsFileName = "vigia_security"
        const val KeyRouterInfo = "lastRouterInfo"
        const val KeyAcknowledged = "acknowledgedFindings"
    }
}
