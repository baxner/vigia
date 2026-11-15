import SwiftUI
import CoreBluetooth
import UIKit

public struct BluetoothView: View {
    @EnvironmentObject private var store: AppStore

    public init() {}

    public var body: some View {
        BluetoothInnerView(bleEngine: store.bleEngine)
    }
}

private struct BluetoothInnerView: View {
    @ObservedObject var bleEngine: RadarBleEngine
    @State private var onlyNamed = false
    @State private var searchText = ""

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                controlsBar
                contentBody
            }
            .background(VigiaColor.background.ignoresSafeArea())
            .navigationTitle("Bluetooth LE")
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $searchText, prompt: "Filtrar por nombre, marca o UUID")
            .onDisappear {
                if bleEngine.huntedPeripheralId == nil {
                    bleEngine.stopScanning()
                }
            }
        }
    }

    private var controlsBar: some View {
        VStack(spacing: 10) {
            HStack {
                Button {
                    if bleEngine.isScanning {
                        bleEngine.stopScanning()
                    } else {
                        bleEngine.startScanning()
                    }
                } label: {
                    Label(
                        bleEngine.isScanning ? "Detener radar BLE" : "Buscar dispositivos BLE",
                        systemImage: bleEngine.isScanning ? "stop.circle.fill" : "dot.radiowaves.left.and.right"
                    )
                    .font(.subheadline.weight(.semibold))
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 10)
                    .background(
                        RoundedRectangle(cornerRadius: 12)
                            .fill(bleEngine.isScanning ? VigiaColor.surfaceElevated : VigiaColor.radarCyan)
                            .overlay(
                                RoundedRectangle(cornerRadius: 12)
                                    .strokeBorder(bleEngine.isScanning ? VigiaColor.risk : .clear, lineWidth: 1)
                            )
                    )
                    .foregroundStyle(bleEngine.isScanning ? VigiaColor.risk : VigiaColor.background)
                    .frame(minHeight: 44)
                }

                Toggle("Con nombre", isOn: $onlyNamed)
                    .toggleStyle(.button)
                    .font(.caption.weight(.semibold))
                    .tint(VigiaColor.electricBlue)
                    .frame(minHeight: 44)
            }

            if bleEngine.bluetoothState == .poweredOff {
                statusAlert(VigiaL10n.tr("El Bluetooth del iPhone está apagado. Enciéndelo en Ajustes o Centro de control."), color: VigiaColor.amber, showSettings: false)
            } else if bleEngine.authorizationState == .denied || bleEngine.authorizationState == .restricted {
                statusAlert(VigiaL10n.tr("Sin permiso de Bluetooth. Habilítalo en Ajustes › Vigía › Bluetooth."), color: VigiaColor.risk, showSettings: true)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
    }

    private func statusAlert(_ text: String, color: Color, showSettings: Bool) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(color)
                Text(text)
                    .font(.caption)
                    .foregroundStyle(VigiaColor.textPrimary)
            }
            if showSettings {
                Button {
                    if let settingsUrl = URL(string: UIApplication.openSettingsURLString) {
                        UIApplication.shared.open(settingsUrl)
                    }
                } label: {
                    Label("Abrir Ajustes de iOS", systemImage: "gear")
                        .font(.caption.weight(.semibold))
                }
                .buttonStyle(.borderedProminent)
                .tint(color)
                .frame(minHeight: 44)
            }
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(color.opacity(0.14)))
    }

    @ViewBuilder
    private var contentBody: some View {
        let items = filteredPeripherals
        if items.isEmpty {
            ScrollView {
                VigiaEmptyState(
                    symbol: "wave.3.right.circle",
                    title: bleEngine.isScanning ? "Escuchando anuncios BLE…" : "Radar Bluetooth listo",
                    message: bleEngine.isScanning
                        ? "Mueve el iPhone lentamente para captar señales cercanas."
                        : "Descubre dispositivos Bluetooth Low Energy cercanos y rastrea su señal con detector frío/caliente y brújula."
                )
                .padding(16)
            }
        } else {
            List {
                ForEach(items) { item in
                    NavigationLink {
                        HuntView(bleEngine: bleEngine, peripheralId: item.id, fallbackTitle: item.displayName)
                    } label: {
                        BlePeripheralRow(reading: item)
                    }
                    .listRowBackground(VigiaColor.surface)
                    .listRowSeparatorTint(VigiaColor.outline)
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
        }
    }

    private var filteredPeripherals: [BlePeripheralReading] {
        let query = searchText.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return bleEngine.peripherals
            .filter { !onlyNamed || $0.isNamed }
            .filter { item in
                guard !query.isEmpty else { return true }
                return item.displayName.lowercased().contains(query)
                    || (item.brandName?.lowercased().contains(query) ?? false)
                    || item.id.uuidString.lowercased().contains(query)
            }
    }
}

private struct BlePeripheralRow: View {
    let reading: BlePeripheralReading

    var body: some View {
        let proxColor = proximityColor(reading.smoothedRssi)
        HStack(spacing: 12) {
            VigiaSignalBars(rssi: Int(reading.smoothedRssi.rounded()), activeColor: proxColor)
                .frame(width: 24)

            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(reading.displayName)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(VigiaColor.textPrimary)
                        .lineLimit(1)
                    if let lanName = reading.matchedLanDeviceName {
                        VigiaStatusChip("LAN: \(lanName)", color: VigiaColor.electricBlue)
                    }
                }
                HStack(spacing: 8) {
                    if let brand = reading.brandName, brand != reading.displayName {
                        Text(brand)
                            .font(.caption)
                            .foregroundStyle(VigiaColor.textSecondary)
                    }
                    Text(DistanceModel.zoneLabelEs(rssi: reading.smoothedRssi, txPowerAt1m: Double(reading.txPower ?? -59)))
                        .font(.caption)
                        .foregroundStyle(VigiaColor.textTertiary)
                }
            }

            Spacer()

            VStack(alignment: .trailing, spacing: 2) {
                Text("\(Int(reading.smoothedRssi.rounded())) dBm")
                    .font(.system(.caption, design: .monospaced).weight(.bold))
                    .foregroundStyle(proxColor)
                Text(String(format: "~%.1f m", reading.estimatedMeters))
                    .font(.system(.caption2, design: .monospaced))
                    .foregroundStyle(VigiaColor.textSecondary)
            }
        }
        .padding(.vertical, 4)
    }

    private func proximityColor(_ rssi: Double) -> Color {
        switch rssi {
        case (-62)...: return VigiaColor.radarCyan
        case -78 ..< -62: return VigiaColor.electricBlue
        default: return VigiaColor.textSecondary
        }
    }
}
