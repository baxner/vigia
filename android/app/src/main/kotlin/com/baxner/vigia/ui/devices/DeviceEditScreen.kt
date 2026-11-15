package com.baxner.vigia.ui.devices

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.SuggestionChipDefaults
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.baxner.vigia.R
import com.baxner.vigia.data.history.DeviceCustomization
import com.baxner.vigia.data.history.KnownDevice
import com.baxner.vigia.data.model.DeviceType
import com.baxner.vigia.data.model.displayName
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.VigiaTextField
import com.baxner.vigia.ui.scan.ScanViewModel
import com.baxner.vigia.ui.theme.VigiaColors

/** Sugerencias rápidas de ubicación: tocarlas rellena el campo, se puede editar igual a mano. */
private val LocationSuggestionResIds = listOf(
    R.string.location_living_room,
    R.string.location_bedroom,
    R.string.location_kitchen,
    R.string.location_office,
    R.string.location_garden,
    R.string.location_garage,
)

/**
 * Envoltura fina sobre [DeviceEditScreenContent]: conecta con el
 * [ScanViewModel] real (igual que el resto de pantallas de `ui/devices`).
 */
@Composable
fun DeviceEditScreen(
    id: Long,
    scanViewModel: ScanViewModel,
    onDone: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val device by scanViewModel.device(id).collectAsStateWithLifecycle(initialValue = null)
    DeviceEditScreenContent(
        device = device,
        onSave = { custom ->
            scanViewModel.customize(id, custom)
            onDone()
        },
        onCancel = onDone,
        modifier = modifier,
    )
}

/**
 * Pantalla Editar: nombre, tipo (abre [DeviceTypePickerDialog]), notas,
 * ubicación (con sugerencias) e "Importante". "Guardar" manda un
 * [DeviceCustomization] completo: un campo vacío vuelve a automático.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DeviceEditScreenContent(
    device: KnownDevice?,
    onSave: (DeviceCustomization) -> Unit,
    onCancel: () -> Unit,
    modifier: Modifier = Modifier,
) {
    if (device == null) {
        Scaffold(modifier = modifier, containerColor = VigiaColors.Background) { padding ->
            EmptyState(
                title = stringResource(R.string.devices_empty),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
        }
        return
    }

    var name by rememberSaveable(device.id) { mutableStateOf(device.custom.name.orEmpty()) }
    var typeId by rememberSaveable(device.id) { mutableStateOf(device.custom.typeId) }
    var notes by rememberSaveable(device.id) { mutableStateOf(device.custom.notes.orEmpty()) }
    var location by rememberSaveable(device.id) { mutableStateOf(device.custom.location.orEmpty()) }
    var important by rememberSaveable(device.id) { mutableStateOf(device.custom.important) }
    var showTypePicker by rememberSaveable { mutableStateOf(false) }

    val engineType = DeviceType.fromRaw(device.last.identity.type)
    val selectedCatalog = DeviceTypeCatalog.find(typeId)
    val typeIcon = selectedCatalog?.icon ?: engineType.icon()
    val typeLabel = selectedCatalog?.label() ?: stringResource(R.string.edit_type_automatic_of, engineType.label())
    val locationSuggestions = LocationSuggestionResIds.map { stringResource(it) }

    fun save() {
        onSave(
            DeviceCustomization(
                name = name.trim().takeUnless { it.isBlank() },
                typeId = typeId,
                notes = notes.trim().takeUnless { it.isBlank() },
                location = location.trim().takeUnless { it.isBlank() },
                important = important,
            ),
        )
    }

    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.edit_title)) },
                navigationIcon = {
                    IconButton(onClick = onCancel) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                actions = {
                    TextButton(onClick = ::save) {
                        Text(
                            stringResource(R.string.edit_save),
                            color = VigiaColors.RadarCyan,
                            fontWeight = FontWeight.SemiBold,
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = VigiaColors.Background,
                    titleContentColor = VigiaColors.TextPrimary,
                ),
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item(key = "name") {
                SectionCard(title = stringResource(R.string.edit_name_title)) {
                    VigiaTextField(
                        value = name,
                        onValueChange = { name = it },
                        placeholder = device.last.displayName(),
                    )
                }
            }
            item(key = "type") {
                SectionCard(title = stringResource(R.string.edit_type_title)) {
                    TypeFieldRow(icon = typeIcon, label = typeLabel, onClick = { showTypePicker = true })
                }
            }
            item(key = "notes") {
                SectionCard(title = stringResource(R.string.edit_notes_title)) {
                    VigiaTextField(
                        value = notes,
                        onValueChange = { notes = it },
                        placeholder = stringResource(R.string.edit_notes_placeholder),
                        singleLine = false,
                        minLines = 3,
                    )
                }
            }
            item(key = "location") {
                SectionCard(title = stringResource(R.string.edit_location_title)) {
                    VigiaTextField(
                        value = location,
                        onValueChange = { location = it },
                        placeholder = stringResource(R.string.edit_location_placeholder),
                    )
                    LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        items(locationSuggestions) { suggestion ->
                            SuggestionChip(
                                onClick = { location = suggestion },
                                label = { Text(suggestion) },
                                colors = SuggestionChipDefaults.suggestionChipColors(
                                    containerColor = VigiaColors.SurfaceElevated,
                                    labelColor = VigiaColors.TextSecondary,
                                ),
                                border = SuggestionChipDefaults.suggestionChipBorder(
                                    enabled = true,
                                    borderColor = VigiaColors.Outline,
                                ),
                            )
                        }
                    }
                }
            }
            item(key = "important") {
                SectionCard(title = stringResource(R.string.edit_important_title)) {
                    Row(
                        // toggleable en la fila completa (ver AGENTS.md §
                        // accesibilidad): TalkBack anuncia "Vigilar este
                        // equipo, interruptor, activado" y el toque funciona
                        // en toda la fila, no solo en el Switch chico.
                        modifier = Modifier.toggleable(
                            value = important,
                            onValueChange = { important = it },
                            role = Role.Switch,
                        ),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        Text(
                            stringResource(R.string.edit_important_label),
                            style = MaterialTheme.typography.bodyMedium,
                            color = VigiaColors.TextPrimary,
                            modifier = Modifier.weight(1f),
                        )
                        Switch(
                            checked = important,
                            onCheckedChange = null,
                            colors = SwitchDefaults.colors(
                                checkedThumbColor = VigiaColors.Background,
                                checkedTrackColor = VigiaColors.RadarCyan,
                            ),
                        )
                    }
                    Text(
                        stringResource(R.string.edit_important_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = VigiaColors.TextSecondary,
                    )
                }
            }
        }
    }

    if (showTypePicker) {
        DeviceTypePickerDialog(
            selectedId = typeId,
            onSelect = { picked ->
                typeId = picked
                showTypePicker = false
            },
            onDismiss = { showTypePicker = false },
        )
    }
}

/** Fila "Tipo" cerrada: icono + etiqueta actual, con flecha; tocarla abre el selector. */
@Composable
private fun TypeFieldRow(icon: ImageVector, label: String, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            // Fusionada: una sola lectura de TalkBack ("Automático
            // (Televisión)") en vez de icono y texto como paradas sueltas.
            .semantics(mergeDescendants = true) {},
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(icon, contentDescription = null, tint = VigiaColors.RadarCyan)
        Text(label, style = MaterialTheme.typography.bodyLarge, color = VigiaColors.TextPrimary, modifier = Modifier.weight(1f))
        Icon(Icons.Default.ChevronRight, contentDescription = null, tint = VigiaColors.TextTertiary)
    }
}
