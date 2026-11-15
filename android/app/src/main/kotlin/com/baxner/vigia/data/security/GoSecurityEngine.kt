package com.baxner.vigia.data.security

import com.baxner.vigia.data.agent.RouterInfo
import com.baxner.vigia.data.agent.SecurityReport
import com.baxner.vigia.data.repository.VigiaJson
import com.baxner.vigia.engine.mobile.Mobile
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json

/**
 * El motor de verdad: `core` en Go vía gomobile (`Mobile.securityReport`,
 * `Mobile.routerInfo`, `Mobile.portCatalog`, ver el contrato § "Un solo
 * motor"). Las llamadas bloquean (routerInfo hasta ~6 s de red): quien llama
 * las corre fuera del hilo principal (ver [SecurityRepository]).
 */
object GoSecurityEngine : SecurityEngine {
    override val isAvailable: Boolean = true

    override fun report(input: SecurityInput): SecurityReport? = runCatching {
        VigiaJson.decodeFromString(SecurityReport.serializer(), Mobile.securityReport(encodeEngineInput(input)))
    }.getOrNull()

    override fun routerInfo(gateway: String): RouterInfo? = runCatching {
        Mobile.routerInfo(gateway).takeUnless { it.isBlank() || it == "null" }
            ?.let { VigiaJson.decodeFromString(RouterInfo.serializer(), it) }
    }.getOrNull()

    override fun portCatalog(): List<PortCatalogEntry> = runCatching {
        VigiaJson.decodeFromString(ListSerializer(PortCatalogEntry.serializer()), Mobile.portCatalog())
    }.getOrDefault(emptyList())
}

/**
 * Con los valores por defecto escritos: [VigiaJson] los omite, y el motor en
 * Go lee un campo ausente como su valor cero — `online` ausente sería
 * `false` (ningún equipo "en línea", ningún riesgo contado) y `security`
 * ausente, "" en vez de "open" (sin aviso de Wi-Fi abierta).
 */
private val EngineJson = Json(VigiaJson) { encodeDefaults = true }

internal fun encodeEngineInput(input: SecurityInput): String =
    EngineJson.encodeToString(SecurityInput.serializer(), input)
