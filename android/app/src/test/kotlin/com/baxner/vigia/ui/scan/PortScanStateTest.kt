package com.baxner.vigia.ui.scan

import androidx.work.Data
import androidx.work.WorkInfo
import com.baxner.vigia.data.history.NetworkKey
import com.baxner.vigia.data.repository.PortScanWork
import java.time.Instant
import java.util.UUID
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

/** De lo que guarda WorkManager a lo que pinta el detalle (ver [portScanStateOf]). */
class PortScanStateTest {

    private val finishedAt: Instant = Instant.parse("2026-10-07T11:20:00Z")

    private fun info(state: WorkInfo.State, out: Data = Data.EMPTY) =
        WorkInfo(id = UUID.randomUUID(), state = state, tags = emptySet(), outputData = out)

    @Test
    fun `sin trabajos guardados no hay escaneo`() {
        assertEquals(PortScanState.Idle, portScanStateOf(emptyList()))
    }

    @Test
    fun `en cola, bloqueado o corriendo es escaneando`() {
        for (state in listOf(WorkInfo.State.ENQUEUED, WorkInfo.State.BLOCKED, WorkInfo.State.RUNNING)) {
            assertEquals(state.name, PortScanState.Scanning, portScanStateOf(listOf(info(state))))
        }
    }

    @Test
    fun `terminado trae los puertos, la IP y la hora`() {
        val out = PortScanWork.successData("192.168.0.55", listOf(139, 445, 3389), finishedAt.toEpochMilli())

        assertEquals(
            PortScanState.Done("192.168.0.55", listOf(139, 445, 3389), finishedAt),
            portScanStateOf(listOf(info(WorkInfo.State.SUCCEEDED, out))),
        )
    }

    @Test
    fun `terminado sin puertos abiertos es lista vacia, no error`() {
        val out = PortScanWork.successData("192.168.0.20", emptyList(), finishedAt.toEpochMilli())

        assertEquals(
            PortScanState.Done("192.168.0.20", emptyList(), finishedAt),
            portScanStateOf(listOf(info(WorkInfo.State.SUCCEEDED, out))),
        )
    }

    @Test
    fun `fallido trae el mensaje`() {
        val out = PortScanWork.failureData("192.168.0.55", "Se interrumpió; vuelve a intentarlo.")

        assertEquals(
            PortScanState.Failed("192.168.0.55", "Se interrumpió; vuelve a intentarlo."),
            portScanStateOf(listOf(info(WorkInfo.State.FAILED, out))),
        )
    }

    @Test
    fun `cancelado vuelve a reposo`() {
        assertEquals(PortScanState.Idle, portScanStateOf(listOf(info(WorkInfo.State.CANCELLED))))
    }

    @Test
    fun `uno en curso manda sobre un resultado anterior`() {
        val anterior = info(WorkInfo.State.SUCCEEDED, PortScanWork.successData("192.168.0.55", listOf(80), finishedAt.toEpochMilli()))

        assertEquals(PortScanState.Scanning, portScanStateOf(listOf(anterior, info(WorkInfo.State.RUNNING))))
    }

    @Test
    fun `el nombre unico separa equipos y historiales`() {
        val local = NetworkKey.of("Casa", "192.168.0.4", "192.168.0.0/24")
        val agente = NetworkKey("192.168.0.4|192.168.0.0/24")

        assertNotEquals(PortScanWork.uniqueName(local, 12), PortScanWork.uniqueName(local, 13))
        assertNotEquals(PortScanWork.uniqueName(local, 12), PortScanWork.uniqueName(agente, 12))
    }
}
