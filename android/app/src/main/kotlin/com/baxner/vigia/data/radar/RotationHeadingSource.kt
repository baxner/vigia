package com.baxner.vigia.data.radar

import android.content.Context
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
import android.os.Build
import android.view.Surface
import android.view.WindowManager
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow

/**
 * Brújula del "Radar con brújula" (fase 6, ver docs/plan.md): vector de
 * rotación a ~20 Hz, compensado por rotación de pantalla y por postura
 * (ver HeadingMath.kt para las fórmulas, probadas aparte con JUnit plano).
 * Solo lee sensores mientras alguien colecta [headings]; cancelar la
 * colección desregistra el listener.
 */
class RotationHeadingSource(private val context: Context) : HeadingSource {
    private val sensorManager =
        context.applicationContext.getSystemService(Context.SENSOR_SERVICE) as? SensorManager

    override fun headings(): Flow<HeadingSample> = callbackFlow {
        val manager = sensorManager
        val sensor = manager?.let(::pickRotationSensor)
        if (manager == null || sensor == null) {
            close()
            return@callbackFlow
        }

        var previousAzimuth: Float? = null
        // Alta por defecto: más vale no asustar con "calibra la brújula" antes
        // de la primera lectura real de onAccuracyChanged.
        var accuracy = SensorManager.SENSOR_STATUS_ACCURACY_HIGH

        val listener = object : SensorEventListener {
            override fun onSensorChanged(event: SensorEvent) {
                val rotationMatrix = FloatArray(9)
                SensorManager.getRotationMatrixFromVector(rotationMatrix, event.values)

                // screenRemapAxes da los mismos valores que SensorManager.AXIS_*
                // (ver HeadingMath.kt): el remapeo no toca el eje Z, así que
                // verticalAzimuthDeg (que lee R[2]/R[5]) sigue siendo válido
                // sobre la matriz ya remapeada.
                val (axisX, axisY) = screenRemapAxes(screenRotation())
                val remapped = FloatArray(9)
                val ok = runCatching {
                    SensorManager.remapCoordinateSystem(rotationMatrix, axisX, axisY, remapped)
                }.isSuccess
                val matrix = if (ok) remapped else rotationMatrix

                val raw = postureAwareAzimuthDeg(matrix)
                val smoothed = smoothAzimuthDeg(previousAzimuth, raw, SmoothingAlpha)
                previousAzimuth = smoothed
                trySend(
                    HeadingSample(
                        atMillis = System.currentTimeMillis(),
                        azimuthDeg = smoothed,
                        reliable = accuracy != SensorManager.SENSOR_STATUS_UNRELIABLE,
                    ),
                )
            }

            override fun onAccuracyChanged(changedSensor: Sensor, newAccuracy: Int) {
                accuracy = newAccuracy
            }
        }

        val registered = manager.registerListener(listener, sensor, SamplingPeriodUs)
        if (!registered) {
            close()
            return@callbackFlow
        }
        awaitClose { manager.unregisterListener(listener) }
    }

    /** Vector de rotación "real" (fusiona giroscopio); si el teléfono no lo trae, el geomagnético (sin giroscopio, más lento para girar). */
    private fun pickRotationSensor(manager: SensorManager): Sensor? =
        manager.getDefaultSensor(Sensor.TYPE_ROTATION_VECTOR)
            ?: manager.getDefaultSensor(Sensor.TYPE_GEOMAGNETIC_ROTATION_VECTOR)

    /**
     * `Context.getDisplay()` (API 30+) solo funciona con un contexto asociado
     * a una pantalla (una Activity, no el contexto de la aplicación): si
     * [context] es el de la aplicación, falla y se asume ROTATION_0 (sin
     * compensar) en vez de tumbar el sensor por esto. Quien use esta fuente
     * desde la pantalla de "Buscar señal" debería pasar el contexto de la
     * Activity, no el de la aplicación, para que la compensación funcione.
     */
    private fun screenRotation(): Int = runCatching {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            context.display?.rotation
        } else {
            @Suppress("DEPRECATION") // getSystemService(WINDOW_SERVICE).defaultDisplay: sin reemplazo simple compatible con minSdk 26 antes de Context.getDisplay() (API 30).
            (context.getSystemService(Context.WINDOW_SERVICE) as? WindowManager)?.defaultDisplay?.rotation
        }
    }.getOrNull() ?: Surface.ROTATION_0

    private companion object {
        // ~20 Hz, como pide el encargo.
        const val SamplingPeriodUs = 50_000

        // Con lecturas a 20 Hz (cada 50 ms), 0.3 responde en unos cientos de ms
        // sin que el número salte a cada ruido del sensor.
        const val SmoothingAlpha = 0.3f
    }
}
