package com.baxner.vigia.ui.scan

import com.baxner.vigia.data.agent.AgentPanelState
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Ensures phone auto-identification never labels a remote LAN's device as this phone. */
class AutoIdentifyPhoneTest {

    private val linkedLocal = AgentPanelState.Linked(
        agentName = "Vigia-Pi",
        reachable = true,
        monitoringCurrentNetwork = true,
        lastScanAt = null,
        viewingRemote = false,
        remoteAvailable = false,
    )

    private val linkedRemote = linkedLocal.copy(
        monitoringCurrentNetwork = false,
        viewingRemote = true,
        remoteAvailable = true,
    )

    @Test
    fun `disabled in demo mode`() {
        assertFalse(shouldAutoIdentifyPhone(demoMode = true, hasActiveRepository = false, panelState = AgentPanelState.NotLinked))
        assertFalse(shouldAutoIdentifyPhone(demoMode = true, hasActiveRepository = true, panelState = linkedLocal))
    }

    @Test
    fun `enabled in local mode without agent repository`() {
        assertTrue(shouldAutoIdentifyPhone(demoMode = false, hasActiveRepository = false, panelState = AgentPanelState.NotLinked))
    }

    @Test
    fun `enabled when linked and monitoring current network`() {
        assertTrue(shouldAutoIdentifyPhone(demoMode = false, hasActiveRepository = true, panelState = linkedLocal))
    }

    @Test
    fun `disabled when viewing remote network`() {
        assertFalse(shouldAutoIdentifyPhone(demoMode = false, hasActiveRepository = true, panelState = linkedRemote))
    }

    @Test
    fun `disabled when active repository exists but monitoringCurrentNetwork is false`() {
        val linkedNotMonitoring = linkedLocal.copy(monitoringCurrentNetwork = false)
        assertFalse(shouldAutoIdentifyPhone(demoMode = false, hasActiveRepository = true, panelState = linkedNotMonitoring))
    }
}
