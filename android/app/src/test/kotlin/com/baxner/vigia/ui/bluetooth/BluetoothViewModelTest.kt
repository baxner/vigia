package com.baxner.vigia.ui.bluetooth

import com.baxner.vigia.data.radar.BleDevice
import com.baxner.vigia.data.radar.FakeBleScanner
import com.baxner.vigia.data.radar.SignalSourceState
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class BluetoothViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private val device = BleDevice(
        address = "02:00:00:00:00:10",
        name = "Tira",
        brand = "Govee",
        rssi = -55,
        txPowerDbm = 0,
        lastSeenMillis = 1_000L,
        randomAddress = false,
    )

    @Test
    fun `al iniciar, refleja los equipos y el estado del escáner`() = runTest(dispatcher) {
        val scanner = FakeBleScanner()
        val vm = BluetoothViewModel(scanner)
        vm.start()
        advanceUntilIdle()

        scanner.setDevices(listOf(device))
        scanner.setState(SignalSourceState.OK)
        advanceUntilIdle()

        assertEquals(listOf(device), vm.uiState.value.devices)
        assertEquals(SignalSourceState.OK, vm.uiState.value.state)
        vm.stop()
    }

    @Test
    fun `detener corta la colecta (nuevos cambios no llegan)`() = runTest(dispatcher) {
        val scanner = FakeBleScanner()
        val vm = BluetoothViewModel(scanner)
        vm.start()
        advanceUntilIdle()
        scanner.setDevices(listOf(device))
        advanceUntilIdle()
        assertEquals(1, vm.uiState.value.devices.size)

        vm.stop()
        scanner.setDevices(emptyList())
        scanner.setDevices(listOf(device, device.copy(address = "otra")))
        advanceUntilIdle()

        // Sin colector activo, el estado se queda como estaba al detener.
        assertEquals(1, vm.uiState.value.devices.size)
    }

    @Test
    fun `el filtro 'solo con nombre' se refleja en el estado`() = runTest(dispatcher) {
        val scanner = FakeBleScanner()
        val vm = BluetoothViewModel(scanner)
        vm.setOnlyNamed(true)
        assertEquals(true, vm.uiState.value.onlyNamed)
    }

    @Test
    fun `onCleared cancela el scope permanentemente`() = runTest(dispatcher) {
        val scanner = FakeBleScanner()
        val vm = BluetoothViewModel(scanner)
        vm.start()
        advanceUntilIdle()
        scanner.setDevices(listOf(device))
        advanceUntilIdle()
        assertEquals(1, vm.uiState.value.devices.size)

        vm.onCleared()
        scanner.setDevices(emptyList())
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.devices.size)
    }
}
