package com.baxner.vigia.ui.internet

import com.baxner.vigia.data.agent.InternetState
import com.baxner.vigia.data.agent.InternetStatusDto
import com.baxner.vigia.data.agent.stateOrUnknown
import kotlin.math.roundToInt

/**
 * Lo que necesita la fila "Internet · 18 ms · bien" de la tarjeta
 * "Vigilancia 24/7" del Panel (ver el encargo, punto 3: "una fila... decide
 * lo más limpio sin saturar", y `ui/agent/AgentCard.kt`). Aparte de
 * [InternetUiState] porque el Panel no necesita la serie, las caídas ni la
 * velocidad — solo el estado y la latencia de ahora mismo.
 */
data class InternetPanelSummary(val state: InternetState, val latencyMs: Int?)

fun InternetStatusDto.toPanelSummary(): InternetPanelSummary =
    InternetPanelSummary(state = stateOrUnknown(), latencyMs = now.internetMs?.roundToInt())
