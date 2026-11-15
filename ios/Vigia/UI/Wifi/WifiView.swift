import SwiftUI
import CoreLocation
import UIKit

public struct WifiView: View {
    @EnvironmentObject private var store: AppStore
    @Binding public var selectedTab: VigiaTab

    public init(selectedTab: Binding<VigiaTab>) {
        self._selectedTab = selectedTab
    }

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 14) {
                    currentWifiCard
                    interfaceCard
                    appleLimitationCard
                }
                .padding(16)
            }
            .background(VigiaColor.background.ignoresSafeArea())
            .navigationTitle("Wi-Fi")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await store.refreshWifi() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                            .vigiaMinTouchTarget()
                    }
                    .accessibilityLabel(Text(VigiaL10n.tr("Actualizar Wi-Fi")))
                }
            }
            .refreshable {
                await store.refreshWifi()
            }
        }
    }

    private var currentWifiCard: some View {
        let wifi = store.wifiSnapshot
        let isOpen = wifi.securityWireValue == "open" || wifi.securityWireValue == "wep"
        let accent: Color = wifi.isAvailable ? (isOpen ? VigiaColor.risk : VigiaColor.radarCyan) : VigiaColor.textSecondary

        return VigiaGlassCard(highlighted: wifi.isAvailable, glowColor: accent) {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Image(systemName: wifi.isAvailable ? "wifi" : "wifi.slash")
                        .font(.title2.weight(.semibold))
                        .foregroundStyle(accent)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(wifi.isAvailable ? wifi.ssid : "Red Wi-Fi no legible todavía")
                            .font(.headline)
                            .foregroundStyle(VigiaColor.textPrimary)
                        Text("Conexión Wi-Fi actual de este iPhone")
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                    Spacer()
                    if wifi.isAvailable {
                        VigiaStatusChip(wifi.securityDisplayLabel, color: accent)
                    }
                }

                if wifi.isAvailable {
                    Divider().background(VigiaColor.outline)
                    if !wifi.bssid.isEmpty {
                        detailRow("BSSID del punto de acceso", value: wifi.bssid.uppercased(), mono: true)
                    }
                    if let signal = wifi.signalStrength {
                        detailRow("Fuerza relativa iOS", value: "\(Int((signal * 100).rounded()))%", mono: true)
                    }
                    if wifi.securityWireValue.isEmpty {
                        Text("iOS agrupa WPA2 Personal y WPA3 Personal bajo la misma etiqueta de sistema; por ello Vigía evita asumir WPA2 en redes WPA3.")
                            .font(.caption2)
                            .foregroundStyle(VigiaColor.textTertiary)
                    }
                } else {
                    Text(wifi.permissionBlocked
                         ? "iOS requiere permiso de Ubicación (Mientras se usa) y la capacidad Access WiFi Information para leer el SSID y BSSID."
                         : "Conéctate a una red Wi-Fi o verifica los permisos de ubicación en Ajustes de iOS.")
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textSecondary)

                    let locStatus = HotspotWifiProvider.shared.locationAuthorizationStatus
                    if locStatus == .denied || locStatus == .restricted {
                        Button {
                            if let settingsUrl = URL(string: UIApplication.openSettingsURLString) {
                                UIApplication.shared.open(settingsUrl)
                            }
                        } label: {
                            Label("Abrir Ajustes de iOS", systemImage: "gear")
                                .font(.subheadline.weight(.semibold))
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(VigiaColor.electricBlue)
                        .frame(minHeight: 44)
                    } else if !HotspotWifiProvider.shared.hasLocationPermissionForWifi {
                        Button {
                            Task { await store.requestLocationForWifi() }
                        } label: {
                            Label("Conceder permiso de Ubicación", systemImage: "location")
                                .font(.subheadline.weight(.semibold))
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(VigiaColor.radarCyan)
                        .foregroundStyle(VigiaColor.background)
                        .frame(minHeight: 44)
                    }
                }
            }
        }
    }

    private var interfaceCard: some View {
        let snap = store.networkSnapshot
        return VigiaGlassCard {
            VStack(alignment: .leading, spacing: 8) {
                Label("Interfaz de red del iPhone", systemImage: "iphone.radiowaves.left.and.right")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.textPrimary)
                detailRow("Interfaz activa", value: snap.interfaceName.isEmpty ? "—" : snap.interfaceName, mono: true)
                detailRow("IPv4 local", value: snap.ipv4.isEmpty ? "—" : snap.ipv4, mono: true)
                detailRow("Máscara", value: snap.netmask.isEmpty ? "—" : snap.netmask, mono: true)
                detailRow("Subred CIDR", value: snap.cidr.isEmpty ? "—" : snap.cidr, mono: true)
                detailRow("Gateway estimado", value: snap.inferredGateway.isEmpty ? "—" : snap.inferredGateway, mono: true)
                detailRow("Túnel VPN", value: snap.isVpnActive ? "Activo" : "No detectado")
            }
        }
    }

    private var appleLimitationCard: some View {
        VigiaGlassCard {
            VStack(alignment: .leading, spacing: 10) {
                Label("Límites transparentes de iOS", systemImage: "info.circle.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(VigiaColor.electricBlue)

                Text(HotspotWifiProvider.iosLimitationExplanation)
                    .font(.caption)
                    .foregroundStyle(VigiaColor.textSecondary)

                HStack(spacing: 10) {
                    Button {
                        selectedTab = .bluetooth
                    } label: {
                        Label("Usar radar Bluetooth", systemImage: "wave.3.right")
                            .font(.caption.weight(.semibold))
                    }
                    .buttonStyle(.bordered)
                    .tint(VigiaColor.radarCyan)

                    Button {
                        selectedTab = .panel
                    } label: {
                        Label("Ir al Panel", systemImage: "dot.radiowaves.left.and.right")
                            .font(.caption.weight(.semibold))
                    }
                    .buttonStyle(.bordered)
                    .tint(VigiaColor.electricBlue)
                }
            }
        }
    }

    private func detailRow(_ title: String, value: String, mono: Bool = false) -> some View {
        HStack {
            Text(title)
                .font(.caption)
                .foregroundStyle(VigiaColor.textSecondary)
            Spacer()
            Text(value)
                .font(mono ? .system(.caption, design: .monospaced) : .caption)
                .foregroundStyle(VigiaColor.textPrimary)
        }
    }
}
