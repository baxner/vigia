// Archivo raíz de build: declara los plugins pero no los aplica aquí (se
// aplican en app/build.gradle.kts). Mantenerlos en el catálogo de versiones
// (gradle/libs.versions.toml) permite que un solo módulo los resuelva.
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.ksp) apply false
}
