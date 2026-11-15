package com.baxner.vigia.data.repository

import kotlinx.serialization.json.Json

/**
 * Instancia compartida de kotlinx.serialization para decodificar lo que
 * manda el motor Go. `ignoreUnknownKeys` porque el motor puede agregar
 * campos a futuro; `coerceInputValues` porque `sources` no tiene `omitempty`
 * en Go y, si el slice es nil, llega como `"sources": null` en vez de
 * ausente — con esto cae en el valor por defecto (lista vacía) en lugar de
 * romper el parseo.
 */
val VigiaJson = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
}
