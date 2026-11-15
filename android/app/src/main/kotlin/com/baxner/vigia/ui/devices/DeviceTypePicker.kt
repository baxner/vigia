package com.baxner.vigia.ui.devices

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.baxner.vigia.R
import com.baxner.vigia.ui.components.VigiaTextField
import com.baxner.vigia.ui.theme.VigiaColors

/**
 * Selector de tipo de equipo, a pantalla completa (Dialog sin el ancho por
 * defecto): catálogo por categorías, buscable, con "Automático" fijo arriba.
 * Se abre desde Editar; [onSelect] manda `null` para volver a automático.
 */
@Composable
fun DeviceTypePickerDialog(
    selectedId: String?,
    onSelect: (String?) -> Unit,
    onDismiss: () -> Unit,
) {
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        DeviceTypePickerContent(selectedId = selectedId, onSelect = onSelect, onClose = onDismiss)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DeviceTypePickerContent(
    selectedId: String?,
    onSelect: (String?) -> Unit,
    onClose: () -> Unit,
) {
    var query by rememberSaveable { mutableStateOf("") }

    Scaffold(
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.type_picker_title)) },
                navigationIcon = {
                    IconButton(onClick = onClose) {
                        Icon(Icons.Default.Close, contentDescription = stringResource(R.string.type_picker_close), tint = VigiaColors.TextPrimary)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = VigiaColors.Background,
                    titleContentColor = VigiaColors.TextPrimary,
                ),
            )
        },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            VigiaTextField(
                value = query,
                onValueChange = { query = it },
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                placeholder = stringResource(R.string.type_picker_search_hint),
                leadingIcon = { Icon(Icons.Default.Search, contentDescription = null, tint = VigiaColors.TextSecondary) },
            )

            val localizedCategories = DeviceTypeCatalog.categories.map { category ->
                val categoryLabel = category.label()
                val entries = DeviceTypeCatalog.entriesFor(category.id).map { entry ->
                    entry to entry.label()
                }.filter { (_, label) ->
                    query.isBlank() || label.contains(query, ignoreCase = true)
                }
                Triple(category.id, categoryLabel, entries)
            }

            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(horizontal = 16.dp, vertical = 4.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                if (query.isBlank()) {
                    item(key = "__auto__") {
                        TypeEntryRow(
                            icon = Icons.Default.AutoAwesome,
                            label = stringResource(R.string.type_picker_automatic),
                            selected = selectedId == null,
                            onClick = { onSelect(null) },
                        )
                    }
                    item(key = "__auto_spacer__") { Spacer(Modifier.height(10.dp)) }
                }

                localizedCategories.forEach { (categoryId, categoryLabel, entries) ->
                    if (entries.isNotEmpty()) {
                        item(key = "cat_$categoryId") {
                            Text(
                                categoryLabel.uppercase(),
                                style = MaterialTheme.typography.labelMedium,
                                color = VigiaColors.TextTertiary,
                                modifier = Modifier.padding(top = 8.dp, bottom = 2.dp, start = 4.dp),
                            )
                        }
                        entries.forEach { (entry, entryLabel) ->
                            item(key = entry.id) {
                                TypeEntryRow(
                                    icon = entry.icon,
                                    label = entryLabel,
                                    selected = selectedId == entry.id,
                                    onClick = { onSelect(entry.id) },
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun TypeEntryRow(icon: ImageVector, label: String, selected: Boolean, onClick: () -> Unit) {
    Row(
        // selectable (no clickable): declara "seleccionado"/"no seleccionado"
        // en la semántica (ver AGENTS.md § accesibilidad) — hoy el tipo
        // elegido solo se distingue por color, borde y la marca de verificación.
        modifier = Modifier
            .fillMaxWidth()
            .selectable(selected = selected, onClick = onClick, role = Role.RadioButton)
            .background(if (selected) VigiaColors.RadarCyan.copy(alpha = 0.12f) else VigiaColors.Surface, RoundedCornerShape(14.dp))
            .border(1.dp, if (selected) VigiaColors.RadarCyan.copy(alpha = 0.5f) else VigiaColors.Outline, RoundedCornerShape(14.dp))
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(icon, contentDescription = null, tint = if (selected) VigiaColors.RadarCyan else VigiaColors.TextSecondary)
        Text(
            label,
            style = MaterialTheme.typography.bodyLarge,
            color = if (selected) VigiaColors.RadarCyan else VigiaColors.TextPrimary,
            modifier = Modifier.weight(1f),
        )
        if (selected) {
            Icon(Icons.Default.Check, contentDescription = null, tint = VigiaColors.RadarCyan, modifier = Modifier.size(18.dp))
        }
    }
}
