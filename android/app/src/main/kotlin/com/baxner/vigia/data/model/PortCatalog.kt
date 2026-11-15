package com.baxner.vigia.data.model

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.baxner.vigia.R

/**
 * Nombres de servicio para puertos TCP conocidos, y la lista de puertos que
 * consideramos "de riesgo" si aparecen abiertos (telnet, SMB, RDP, VNC,
 * RTSP sin auth...).
 */
object PortCatalog {
    private val names: Map<Int, String> = mapOf(
        21 to "FTP",
        22 to "SSH",
        23 to "Telnet",
        53 to "DNS",
        80 to "HTTP",
        139 to "NetBIOS",
        443 to "HTTPS",
        445 to "SMB",
        515 to "Impresora (LPD)",
        548 to "AFP (Mac)",
        554 to "RTSP",
        631 to "IPP (impresora)",
        1900 to "UPnP",
        3389 to "RDP",
        3689 to "iTunes/DAAP",
        5000 to "UPnP/HTTP",
        5353 to "mDNS",
        5900 to "VNC",
        7000 to "AirPlay",
        8001 to "Control de TV Samsung",
        8060 to "Control de Roku",
        8008 to "Chromecast",
        8009 to "Chromecast",
        8080 to "HTTP alt.",
        8443 to "HTTPS alt.",
        9100 to "Impresora (RAW)",
        32400 to "Plex",
        49152 to "UPnP",
        62078 to "Sincronización iPhone",
    )

    private val localizedNameRes: Map<Int, Int> = mapOf(
        515 to R.string.port_name_515,
        631 to R.string.port_name_631,
        8001 to R.string.port_name_8001,
        8060 to R.string.port_name_8060,
        9100 to R.string.port_name_9100,
        62078 to R.string.port_name_62078,
    )

    /** Nombre legible de un puerto, o "Puerto <n>" si no está en el catálogo. */
    fun nameFor(port: Int): String = names[port] ?: "Puerto $port"

    /** Nombre localizado del puerto para la UI Compose. */
    @Composable
    fun localizedNameFor(port: Int): String {
        val res = localizedNameRes[port]
        return when {
            res != null -> stringResource(res)
            port in names -> names.getValue(port)
            else -> stringResource(R.string.port_name_generic, port)
        }
    }

    /** Nombre localizado del puerto usando [Context]. */
    fun nameFor(context: Context, port: Int): String {
        val res = localizedNameRes[port]
        return when {
            res != null -> context.getString(res)
            port in names -> names.getValue(port)
            else -> context.getString(R.string.port_name_generic, port)
        }
    }

    data class RiskyPort(
        val port: Int,
        val serviceName: String,
        val reason: String,
        @StringRes val reasonRes: Int,
    )

    val riskyPorts: List<RiskyPort> = listOf(
        RiskyPort(21, "FTP", "Transferencia de archivos sin cifrar", R.string.port_risk_21),
        RiskyPort(23, "Telnet", "Acceso remoto sin cifrar", R.string.port_risk_23),
        RiskyPort(139, "NetBIOS", "Compartir archivos antiguo de Windows", R.string.port_risk_139),
        RiskyPort(445, "SMB", "Compartir archivos de Windows, blanco frecuente de ataques", R.string.port_risk_445),
        RiskyPort(554, "RTSP", "Stream de cámara, a veces sin autenticación", R.string.port_risk_554),
        RiskyPort(3389, "RDP", "Escritorio remoto, blanco frecuente de fuerza bruta", R.string.port_risk_3389),
        RiskyPort(5900, "VNC", "Escritorio remoto, a veces sin contraseña", R.string.port_risk_5900),
    )

    private val riskyPortNumbers: Set<Int> = riskyPorts.map { it.port }.toSet()

    fun isRisky(port: Int): Boolean = port in riskyPortNumbers

    /** De una lista de puertos abiertos, los que están en nuestro catálogo de riesgo. */
    fun riskyAmong(openPorts: List<Int>): List<RiskyPort> {
        if (openPorts.isEmpty()) return emptyList()
        val open = openPorts.toSet()
        return riskyPorts.filter { it.port in open }
    }
}
