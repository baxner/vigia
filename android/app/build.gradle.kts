plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
}

android {
    namespace = "com.baxner.vigia"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.baxner.vigia"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"

        // El .aar del motor (vigia-core.aar) solo trae librerías nativas para
        // estas dos ABIs (ver android/app/libs/vigia-core.aar/jni/*). Lo
        // declaramos explícito para que el empaquetado no intente buscar
        // otras que no existen.
        ndk {
            abiFilters += listOf("arm64-v8a", "x86_64")
        }
    }

    val keystorePath = System.getenv("VIGIA_KEYSTORE_PATH")?.takeIf { it.isNotBlank() }
    val keystorePassword = System.getenv("VIGIA_KEYSTORE_PASSWORD")?.takeIf { it.isNotBlank() }
    val keyAliasValue = System.getenv("VIGIA_KEY_ALIAS")?.takeIf { it.isNotBlank() }
    val keyPasswordValue = System.getenv("VIGIA_KEY_PASSWORD")?.takeIf { it.isNotBlank() }
    val hasReleaseSigning = keystorePath != null &&
        keystorePassword != null &&
        keyAliasValue != null &&
        keyPasswordValue != null &&
        file(keystorePath).exists()

    signingConfigs {
        if (hasReleaseSigning) {
            create("release") {
                storeFile = file(keystorePath!!)
                storePassword = keystorePassword
                keyAlias = keyAliasValue
                keyPassword = keyPasswordValue
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            if (hasReleaseSigning) {
                signingConfig = signingConfigs.getByName("release")
            }
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }

    testOptions {
        unitTests {
            isReturnDefaultValues = true
            // Roborazzi/Robolectric necesitan los recursos reales (fuentes,
            // strings.xml) para que las capturas de Compose salgan bien.
            isIncludeAndroidResources = true
        }
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

// Room (historial local, ver data/history/): el esquema exportado de cada
// versión queda versionado en el módulo, no en build/, para poder escribir
// migraciones reales contra el JSON de la versión anterior el día que haga falta.
ksp {
    arg("room.schemaLocation", "$projectDir/schemas")
}

// Roborazzi graba PNGs solo cuando se pide explícitamente (`-Proborazzi.test.record=true`
// o `-Droborazzi.test.record=true`); por defecto queda en `false` para que
// `./gradlew testDebugUnitTest` pase limpio sin dejar artefactos PNG temporales.
tasks.withType<Test>().configureEach {
    val recordScreenshots = providers.gradleProperty("roborazzi.test.record")
        .orElse(providers.systemProperty("roborazzi.test.record"))
        .orElse("false")
        .get()
    systemProperty("roborazzi.test.record", recordScreenshots)
}

dependencies {
    // Motor de escaneo (Go vía gomobile). Ver android/app/libs/vigia-core.aar.
    implementation(files("libs/vigia-core.aar"))

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.navigation.compose)

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.material.icons.extended)
    debugImplementation(libs.androidx.compose.ui.tooling)

    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)

    // Historial local (ver data/history/): Room + su extensión de corrutinas (withTransaction).
    implementation(libs.androidx.room.runtime)
    implementation(libs.androidx.room.ktx)
    ksp(libs.androidx.room.compiler)

    // WorkManager: trabajo que debe terminar (o repetirse) aunque se salga de
    // la app, p. ej. el escaneo completo de puertos de un equipo.
    implementation(libs.androidx.work.runtime)

    // Widget de pantalla de inicio (fase 8, ver docs/plan.md y ui/widget/):
    // Jetpack Glance, no RemoteViews a mano.
    implementation(libs.androidx.glance.appwidget)

    // Agente 24/7 (ver data/agent/ y docs/agente-api.md): cliente HTTP + SSE
    // hacia el agente, y guardado cifrado del vínculo (token).
    implementation(libs.okhttp)
    implementation(libs.androidx.security.crypto)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.okhttp.tls)

    // Revisión visual (sin emulador): capturas de Compose en la JVM con
    // Roborazzi + Robolectric. Van en línea (no en el catálogo de versiones)
    // porque solo las usan las pruebas de pantalla.
    testImplementation("org.robolectric:robolectric:4.17")
    testImplementation("androidx.test:core:1.7.0")
    testImplementation("androidx.test.ext:junit:1.3.0")
    testImplementation(platform(libs.androidx.compose.bom))
    testImplementation("androidx.compose.ui:ui-test-junit4")
    debugImplementation(platform(libs.androidx.compose.bom))
    debugImplementation("androidx.compose.ui:ui-test-manifest")
    // Fijo en 1.60.0 (no la última): de ahí para arriba Roborazzi compila con
    // Kotlin 2.3.x y su metadata binaria no la lee nuestro Kotlin 2.1.20
    // ("Module was compiled with an incompatible version of Kotlin").
    testImplementation("io.github.takahirom.roborazzi:roborazzi:1.60.0")
    testImplementation("io.github.takahirom.roborazzi:roborazzi-compose:1.60.0")
}
