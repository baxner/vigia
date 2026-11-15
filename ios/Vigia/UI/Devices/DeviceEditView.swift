import SwiftUI

public struct DeviceEditView: View {
    @EnvironmentObject private var store: AppStore
    @Environment(\.dismiss) private var dismiss

    public let device: KnownDevice

    @State private var name: String
    @State private var selectedType: DeviceType?
    @State private var location: String
    @State private var notes: String
    @State private var important: Bool
    @State private var isSaving = false

    public init(device: KnownDevice) {
        self.device = device
        _name = State(initialValue: device.custom.name ?? "")
        let rawCustomType = device.custom.typeId?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        _selectedType = State(initialValue: rawCustomType.isEmpty ? nil : DeviceType.fromRaw(rawCustomType))
        _location = State(initialValue: device.custom.location ?? "")
        _notes = State(initialValue: device.custom.notes ?? "")
        _important = State(initialValue: device.custom.important)
    }

    public var body: some View {
        NavigationStack {
            Form {
                Section("Identidad") {
                    TextField("Nombre personalizado (opcional)", text: $name)
                    TextField("Ubicación (ej. Sala, Estudio)", text: $location)
                    Toggle("Marcar como equipo importante", isOn: $important)
                }

                Section("Tipo de equipo") {
                    Button {
                        selectedType = nil
                    } label: {
                        HStack {
                            Label("Automático (\(DeviceType.fromRaw(device.last.identity.type).labelEs))", systemImage: "wand.and.stars")
                            Spacer()
                            if selectedType == nil {
                                Image(systemName: "checkmark")
                                    .foregroundStyle(VigiaColor.radarCyan)
                            }
                        }
                    }

                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 100), spacing: 8)], spacing: 8) {
                        ForEach(DeviceType.allCases.filter { $0 != .unknown }) { type in
                            let chosen = selectedType == type
                            Button {
                                selectedType = type
                            } label: {
                                VStack(spacing: 5) {
                                    Image(systemName: type.sfSymbol)
                                        .font(.title3)
                                    Text(type.labelEs)
                                        .font(.caption2)
                                        .lineLimit(1)
                                }
                                .frame(maxWidth: .infinity, minHeight: 54)
                                .padding(.vertical, 10)
                                .background(
                                    RoundedRectangle(cornerRadius: 12)
                                        .fill(chosen ? VigiaColor.radarCyan.opacity(0.2) : VigiaColor.surfaceElevated)
                                        .overlay(
                                            RoundedRectangle(cornerRadius: 12)
                                                .strokeBorder(chosen ? VigiaColor.radarCyan : VigiaColor.outline, lineWidth: 1)
                                        )
                                )
                                .foregroundStyle(chosen ? VigiaColor.radarCyan : VigiaColor.textSecondary)
                            }
                            .buttonStyle(.plain)
                            .accessibilityAddTraits(chosen ? [.isSelected] : [])
                        }
                    }
                    .padding(.vertical, 6)
                }

                Section("Notas") {
                    TextEditor(text: $notes)
                        .frame(minHeight: 90)
                }
            }
            .scrollContentBackground(.hidden)
            .background(VigiaColor.background.ignoresSafeArea())
            .preferredColorScheme(.dark)
            .navigationTitle("Editar equipo")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancelar") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button {
                        save()
                    } label: {
                        if isSaving {
                            ProgressView()
                        } else {
                            Text("Guardar")
                        }
                    }
                    .disabled(isSaving)
                }
            }
        }
    }

    private func save() {
        isSaving = true
        let trimmedName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedLoc = location.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedNotes = notes.trimmingCharacters(in: .whitespacesAndNewlines)

        let custom = DeviceCustomization(
            name: trimmedName.isEmpty ? nil : trimmedName,
            typeId: selectedType?.rawValue,
            notes: trimmedNotes.isEmpty ? nil : trimmedNotes,
            location: trimmedLoc.isEmpty ? nil : trimmedLoc,
            important: important
        )

        Task {
            _ = await store.updateDeviceCustomization(deviceId: device.id, custom: custom)
            isSaving = false
            dismiss()
        }
    }
}
