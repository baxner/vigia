// EncryptedSharedPreferences/MasterKey: androidx.security-crypto 1.1.0 las marca
// deprecated pero no trae todavía un reemplazo estable (ver EncryptedAgentLinkStore).
@file:Suppress("DEPRECATION")

package com.baxner.vigia.data.agent

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * El vínculo con un agente ya emparejado (ver "Vinculación" en el contrato):
 * quién es, dónde está y el token para hablarle. Un solo vínculo por teléfono
 * a la vez (como pide el encargo), no una lista.
 *
 * [toString] se sobreescribe para que el token nunca aparezca, ni siquiera
 * por accidente (un log de depuración, un reporte de crash que serializa el
 * objeto…): quien de verdad necesite el token lo lee del campo, nunca de
 * `"$link"`.
 */
data class AgentLink(
    val agentId: String,
    val agentName: String,
    val host: String,
    val port: Int,
    val token: String,
    /**
     * Huella SHA-256 (hex) del certificado TLS del agente (ver TLS v1.5,
     * `docs/agente-api.md`), fijada al vincular. `null` en un vínculo
     * guardado antes de esta fase, hasta que se migra (ver
     * [migrateLegacyLinkIfNeeded] en `AgentTls.kt`) — de ahí en más, todas
     * las llamadas van por HTTPS exigiendo exactamente esta huella.
     */
    val certSha256: String? = null,
) {
    override fun toString(): String =
        "AgentLink(agentId=$agentId, agentName=$agentName, host=$host, port=$port, certSha256=$certSha256, token=<redacted>)"
}

/** De dónde sale/se guarda el vínculo. Separado de la implementación cifrada para poder probar el resto del cliente con un fake en memoria. */
interface AgentLinkStore {
    fun get(): AgentLink?
    fun save(link: AgentLink)
    fun clear()
}

/**
 * Persistencia cifrada del vínculo (EncryptedSharedPreferences sobre el
 * Keystore de Android, ver el encargo: "el token nunca en logs" — tampoco en
 * un SharedPreferences plano). Pensado para el dispositivo real: no se prueba
 * con JUnit/Robolectric (el Keystore de verdad no está disponible ahí); la
 * lógica que sí se prueba (selección de modo, cliente HTTP, migración de
 * personalizaciones) recibe un [AgentLinkStore] inyectado — ver los tests con
 * un fake.
 */
class EncryptedAgentLinkStore(context: Context) : AgentLinkStore {
    private val appContext = context.applicationContext

    private fun createEncryptedPrefs(): SharedPreferences {
        val masterKey = MasterKey.Builder(appContext)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        return EncryptedSharedPreferences.create(
            appContext,
            PrefsFileName,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    private fun prefsOrNull(): SharedPreferences? {
        val first = runCatching { createEncryptedPrefs() }
        if (first.isSuccess) return first.getOrNull()
        // Si el Keystore cambió (p. ej. tras restaurar un respaldo de nube) el
        // archivo cifrado ya no se puede descifrar: se limpia y se recrea en
        // lugar de crashear al leer/guardar el vínculo.
        return runCatching {
            appContext.deleteSharedPreferences(PrefsFileName)
            createEncryptedPrefs()
        }.getOrNull()
    }

    override fun get(): AgentLink? = runCatching {
        val p = prefsOrNull() ?: return@runCatching null
        val agentId = p.getString(KeyAgentId, null) ?: return@runCatching null
        val agentName = p.getString(KeyAgentName, null) ?: return@runCatching null
        val host = p.getString(KeyHost, null) ?: return@runCatching null
        val port = p.getInt(KeyPort, -1).takeIf { it in 1..65535 } ?: return@runCatching null
        val token = p.getString(KeyToken, null) ?: return@runCatching null
        // Ausente en un vínculo guardado antes de TLS v1.5 (ver AgentLink.certSha256): null, no "".
        val certSha256 = p.getString(KeyCertSha256, null)
        AgentLink(agentId, agentName, host, port, token, certSha256)
    }.getOrNull()

    override fun save(link: AgentLink) {
        runCatching {
            val p = prefsOrNull() ?: return@runCatching
            val editor = p.edit()
                .putString(KeyAgentId, link.agentId)
                .putString(KeyAgentName, link.agentName)
                .putString(KeyHost, link.host)
                .putInt(KeyPort, link.port)
                .putString(KeyToken, link.token)
            if (link.certSha256 != null) editor.putString(KeyCertSha256, link.certSha256) else editor.remove(KeyCertSha256)
            editor.apply()
        }
    }

    override fun clear() {
        runCatching {
            prefsOrNull()?.edit()?.clear()?.apply()
        }
    }

    private companion object {
        const val PrefsFileName = "vigia_agent_link"
        const val KeyAgentId = "agentId"
        const val KeyAgentName = "agentName"
        const val KeyHost = "host"
        const val KeyPort = "port"
        const val KeyToken = "token"
        const val KeyCertSha256 = "certSha256"
    }
}
