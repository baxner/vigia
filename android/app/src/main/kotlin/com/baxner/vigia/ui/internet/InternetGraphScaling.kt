package com.baxner.vigia.ui.internet

import com.baxner.vigia.data.agent.OutageDto
import java.time.Instant
import java.time.OffsetDateTime
import java.time.format.DateTimeParseException

/**
 * Escalado puro de la gráfica de latencia de 24 h / 7 días (ver el encargo:
 * "dibujada a mano con Canvas... línea del tramo internet, con huecos donde
 * no hay dato; marcas rojas en las caídas"). Sin Canvas ni Compose: la UI
 * ([LatencyChart]) solo traduce esto a píxeles, y esto se prueba con JUnit
 * plano.
 */
object LatencyGraphScaling {

    /**
     * Mínimo y máximo de [values] (ignorando huecos `null`). Con menos de un
     * dato útil, un rango de 1 ms centrado en lo que haya (o `0..1` sin
     * ninguno) para que [GraphRange.normalize] no divida por cero.
     */
    fun rangeOf(values: List<Double?>): GraphRange {
        val present = values.filterNotNull()
        if (present.isEmpty()) return GraphRange(0.0, 1.0)
        val min = present.min()
        val max = present.max()
        return if (min == max) GraphRange(min - 1.0, max + 1.0) else GraphRange(min, max)
    }

    /**
     * Tramos contiguos sin huecos (`null`), como listas de índices de
     * [values]: la UI dibuja una línea por tramo en vez de unir dos puntos
     * con un salto que no existió — "huecos donde no hay dato" (ver el
     * encargo).
     */
    fun segments(values: List<Double?>): List<List<Int>> {
        val result = mutableListOf<MutableList<Int>>()
        var current: MutableList<Int>? = null
        values.forEachIndexed { index, value ->
            if (value == null) {
                current = null
            } else {
                val segment = current ?: mutableListOf<Int>().also {
                    current = it
                    result += it
                }
                segment += index
            }
        }
        return result
    }

    /**
     * Índices de [timestamps] (el `at` de cada punto de la serie, RFC 3339)
     * que caen dentro de alguna caída de [outages] — "marcas rojas en las
     * caídas" (ver el encargo). Una caída en curso ([OutageDto.end] `null`)
     * se extiende hasta [now]. Un timestamp que no se puede parsear se
     * ignora (nunca revienta).
     */
    fun outageMarkerIndices(timestamps: List<String>, outages: List<OutageDto>, now: Instant = Instant.now()): Set<Int> {
        if (outages.isEmpty()) return emptySet()
        val ranges = outages.mapNotNull { outage ->
            val start = parseInstantOrNull(outage.start) ?: return@mapNotNull null
            val end = outage.end?.let(::parseInstantOrNull) ?: now
            start to end
        }
        if (ranges.isEmpty()) return emptySet()
        val marked = mutableSetOf<Int>()
        timestamps.forEachIndexed { index, raw ->
            val at = parseInstantOrNull(raw) ?: return@forEachIndexed
            if (ranges.any { (start, end) -> !at.isBefore(start) && !at.isAfter(end) }) marked += index
        }
        return marked
    }

    private fun parseInstantOrNull(rfc3339: String): Instant? =
        try {
            OffsetDateTime.parse(rfc3339).toInstant()
        } catch (_: DateTimeParseException) {
            null
        }
}

/**
 * [min]/[max] de un tramo de la gráfica (ver [LatencyGraphScaling.rangeOf]).
 * [normalize] lleva un valor a `0f` (abajo, [min]) .. `1f` (arriba, [max]).
 */
data class GraphRange(val min: Double, val max: Double) {
    fun normalize(value: Double): Float {
        if (max <= min) return 0.5f
        return ((value - min) / (max - min)).toFloat().coerceIn(0f, 1f)
    }
}
