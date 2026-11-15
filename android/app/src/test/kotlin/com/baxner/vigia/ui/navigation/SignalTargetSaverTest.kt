package com.baxner.vigia.ui.navigation

import androidx.compose.runtime.saveable.SaverScope
import com.baxner.vigia.data.radar.SignalTarget
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Verifies state restoration across process death / configuration change for [SignalTargetSaver]. */
class SignalTargetSaverTest {

    private val scope = SaverScope { true }

    @Test
    fun `null round-trips as null`() {
        val saved = with(SignalTargetSaver) { scope.save(null) }
        assertEquals("", saved)
        assertNull(SignalTargetSaver.restore(saved!!))
        assertNull(SignalTargetSaver.restore(emptyList<Any>()))
    }

    @Test
    fun `wifi target round-trips all fields`() {
        val original = SignalTarget.WifiAp(
            bssid = "aa:bb:cc:dd:ee:ff",
            ssid = "CasaNet",
            frequencyMhz = 5180,
            rttCapable = true,
            connected = false,
            label = "CasaNet · 5 GHz",
        )
        val saved = with(SignalTargetSaver) { scope.save(original) }!!
        val restored = SignalTargetSaver.restore(saved)
        assertEquals(original, restored)
    }

    @Test
    fun `ble target round-trips all fields including nullable properties`() {
        val original = SignalTarget.Ble(
            address = "11:22:33:44:55:66",
            name = null,
            brand = "Govee",
            txPowerDbm = null,
            label = "Govee BLE",
        )
        val saved = with(SignalTargetSaver) { scope.save(original) }!!
        val restored = SignalTargetSaver.restore(saved)
        assertEquals(original, restored)
    }
}
