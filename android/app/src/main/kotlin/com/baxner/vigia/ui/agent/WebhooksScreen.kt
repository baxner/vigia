package com.baxner.vigia.ui.agent

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.AlertType
import com.baxner.vigia.data.agent.WebhookDto
import com.baxner.vigia.ui.alerts.labelRes
import com.baxner.vigia.ui.components.EmptyState
import com.baxner.vigia.ui.components.GlassSurface
import com.baxner.vigia.ui.components.VigiaTextField
import com.baxner.vigia.ui.theme.VigiaColors

/**
 * "Webhooks" dentro de Ajustes del agente (ver el encargo de la fase 8,
 * "Qué construir", punto 2): listar, agregar, borrar y "Probar". Ver
 * [WebhooksScreenContent] para la versión sin estado (capturas, tests).
 */
@Composable
fun WebhooksScreen(onBack: () -> Unit, modifier: Modifier = Modifier) {
    val viewModel: WebhooksViewModel = viewModel()
    val uiState by viewModel.uiState.collectAsStateWithLifecycle()
    val form by viewModel.form.collectAsStateWithLifecycle()
    var showAddSheet by rememberSaveable { mutableStateOf(false) }

    LaunchedEffect(Unit) { viewModel.onOpen() }

    WebhooksScreenContent(
        uiState = uiState,
        onBack = onBack,
        onAddClick = { viewModel.resetForm(); showAddSheet = true },
        onDelete = viewModel::delete,
        onTest = viewModel::test,
        modifier = modifier,
    )

    if (showAddSheet) {
        AddWebhookSheet(
            form = form,
            onUrlChange = viewModel::setFormUrl,
            onSecretChange = viewModel::setFormSecret,
            onToggleType = viewModel::toggleFormType,
            onDismiss = { showAddSheet = false },
            onSubmit = { viewModel.submitForm(onAdded = { showAddSheet = false }) },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WebhooksScreenContent(
    uiState: WebhooksUiState,
    onBack: () -> Unit,
    onAddClick: () -> Unit,
    modifier: Modifier = Modifier,
    onDelete: (Long) -> Unit = {},
    onTest: (Long) -> Unit = {},
) {
    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.webhooks_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = VigiaColors.Background, titleContentColor = VigiaColors.TextPrimary),
            )
        },
        floatingActionButton = {
            if (uiState.linked) {
                FloatingActionButton(onClick = onAddClick, containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background) {
                    Icon(Icons.Default.Add, contentDescription = stringResource(R.string.webhooks_add_button))
                }
            }
        },
    ) { padding ->
        if (!uiState.linked) {
            EmptyState(
                title = stringResource(R.string.webhooks_empty_not_linked_title),
                subtitle = stringResource(R.string.webhooks_empty_not_linked_subtitle),
                modifier = Modifier.fillMaxSize().padding(padding),
            )
            return@Scaffold
        }

        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item(key = "explanation") {
                Text(
                    stringResource(R.string.webhooks_explanation),
                    style = MaterialTheme.typography.bodySmall,
                    color = VigiaColors.TextSecondary,
                )
            }
            if (uiState.loadError != null) {
                item(key = "error") {
                    Text(
                        stringResource(R.string.webhooks_load_error, uiState.loadError),
                        style = MaterialTheme.typography.bodySmall,
                        color = VigiaColors.Risk,
                    )
                }
            }
            if (uiState.webhooks.isEmpty() && !uiState.loading) {
                item(key = "empty") {
                    EmptyState(
                        title = stringResource(R.string.webhooks_empty_title),
                        subtitle = stringResource(R.string.webhooks_empty_subtitle),
                    )
                }
            } else {
                items(uiState.webhooks, key = { it.id }) { webhook ->
                    WebhookRow(
                        webhook = webhook,
                        testOutcome = uiState.testResults[webhook.id],
                        busy = uiState.pendingActionId == webhook.id,
                        onDelete = { onDelete(webhook.id) },
                        onTest = { onTest(webhook.id) },
                    )
                }
            }
        }
    }
}

@Composable
private fun WebhookRow(
    webhook: WebhookDto,
    testOutcome: WebhookTestOutcome?,
    busy: Boolean,
    onDelete: () -> Unit,
    onTest: () -> Unit,
) {
    var showDeleteConfirm by remember { mutableStateOf(false) }

    GlassSurface(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(webhook.url, style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextPrimary)
            Text(
                if (webhook.types.isEmpty()) stringResource(R.string.webhooks_types_all) else webhook.types.joinToString(" · "),
                style = MaterialTheme.typography.bodySmall,
                color = VigiaColors.TextSecondary,
            )
            Text(lastStatusLabel(webhook), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
            testOutcome?.let { outcome ->
                val text = when (outcome) {
                    is WebhookTestOutcome.Succeeded -> stringResource(R.string.webhooks_test_result_ok, outcome.status)
                    is WebhookTestOutcome.Failed -> stringResource(R.string.webhooks_test_result_error, outcome.message)
                }
                val color = if (outcome is WebhookTestOutcome.Succeeded) VigiaColors.Ok else VigiaColors.Risk
                Text(text, style = MaterialTheme.typography.bodySmall, color = color)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                if (busy) {
                    CircularProgressIndicator(modifier = Modifier.padding(8.dp), color = VigiaColors.RadarCyan)
                } else {
                    TextButton(onClick = onTest) {
                        Icon(Icons.Default.PlayArrow, contentDescription = null, tint = VigiaColors.RadarCyan)
                        Text(stringResource(R.string.webhooks_test_button), color = VigiaColors.RadarCyan)
                    }
                    IconButton(onClick = { showDeleteConfirm = true }) {
                        Icon(Icons.Default.Delete, contentDescription = stringResource(R.string.webhooks_delete), tint = VigiaColors.Risk)
                    }
                }
            }
        }
    }

    if (showDeleteConfirm) {
        AlertDialog(
            onDismissRequest = { showDeleteConfirm = false },
            title = { Text(stringResource(R.string.webhooks_delete_confirm_title)) },
            text = { Text(stringResource(R.string.webhooks_delete_confirm_message, webhook.url)) },
            confirmButton = {
                TextButton(onClick = { showDeleteConfirm = false; onDelete() }) {
                    Text(stringResource(R.string.webhooks_delete), color = VigiaColors.Risk)
                }
            },
            dismissButton = {
                TextButton(onClick = { showDeleteConfirm = false }) { Text(stringResource(R.string.webhooks_cancel)) }
            },
            containerColor = VigiaColors.SurfaceElevated,
            titleContentColor = VigiaColors.TextPrimary,
            textContentColor = VigiaColors.TextSecondary,
        )
    }
}

@Composable
private fun lastStatusLabel(webhook: WebhookDto): String = when {
    webhook.lastError != null -> stringResource(R.string.webhooks_last_status_error, webhook.lastError)
    webhook.lastStatus != null -> stringResource(R.string.webhooks_last_status_ok, webhook.lastStatus)
    else -> stringResource(R.string.webhooks_last_status_none)
}

/**
 * Los campos del formulario, sin el `AlertDialog` que los envuelve: separado
 * para que `screenshot/WebhooksScreenshotTest.kt` pueda capturarlo directo.
 * Un `AlertDialog` de Compose abre su propia ventana con una animación de
 * entrada que Robolectric (con Roborazzi en `GraphicsMode.NATIVE`) nunca
 * termina de asentar — el mismo motivo por el que ninguna otra pantalla de
 * la app intenta capturar uno (ver `AlertsScreen.kt`/`AgentSettingsScreen.kt`,
 * que también usan `AlertDialog` pero sin capturarlo).
 */
@Composable
internal fun AddWebhookFormFields(
    form: AddWebhookFormState,
    onUrlChange: (String) -> Unit,
    onSecretChange: (String) -> Unit,
    onToggleType: (String, Boolean) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            VigiaTextField(
                value = form.url,
                onValueChange = onUrlChange,
                placeholder = stringResource(R.string.webhooks_url_placeholder),
            )
            if (form.error == WebhooksViewModel.InvalidUrlMarker) {
                Text(stringResource(R.string.webhooks_url_error), style = MaterialTheme.typography.bodySmall, color = VigiaColors.Risk)
            } else if (form.error != null) {
                Text(form.error, style = MaterialTheme.typography.bodySmall, color = VigiaColors.Risk)
            }
        }
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(stringResource(R.string.webhooks_types_label), style = MaterialTheme.typography.labelMedium, color = VigiaColors.TextPrimary)
            Text(stringResource(R.string.webhooks_types_hint), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
            AlertType.entries.forEach { type ->
                TypeToggleRow(
                    label = stringResource(type.labelRes()),
                    checked = type.name in form.selectedTypes,
                    onCheckedChange = { enabled -> onToggleType(type.name, enabled) },
                )
            }
        }
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            VigiaTextField(
                value = form.secret,
                onValueChange = onSecretChange,
                placeholder = stringResource(R.string.webhooks_secret_placeholder),
            )
            Text(stringResource(R.string.webhooks_secret_hint), style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextTertiary)
        }
    }
}

@Composable
private fun AddWebhookSheet(
    form: AddWebhookFormState,
    onUrlChange: (String) -> Unit,
    onSecretChange: (String) -> Unit,
    onToggleType: (String, Boolean) -> Unit,
    onDismiss: () -> Unit,
    onSubmit: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.webhooks_add_title)) },
        text = {
            AddWebhookFormFields(
                form = form,
                onUrlChange = onUrlChange,
                onSecretChange = onSecretChange,
                onToggleType = onToggleType,
            )
        },
        confirmButton = {
            Button(
                onClick = onSubmit,
                enabled = !form.submitting,
                colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background),
            ) {
                Text(if (form.submitting) stringResource(R.string.webhooks_saving) else stringResource(R.string.webhooks_save))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.webhooks_cancel)) }
        },
        containerColor = VigiaColors.SurfaceElevated,
        titleContentColor = VigiaColors.TextPrimary,
        textContentColor = VigiaColors.TextSecondary,
    )
}

@Composable
private fun TypeToggleRow(label: String, checked: Boolean, onCheckedChange: (Boolean) -> Unit) {
    Row(
        modifier = Modifier.toggleable(value = checked, onValueChange = onCheckedChange, role = Role.Switch),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(label, style = MaterialTheme.typography.bodySmall, color = VigiaColors.TextPrimary, modifier = Modifier.weight(1f))
        Switch(
            checked = checked,
            onCheckedChange = null,
            colors = SwitchDefaults.colors(checkedThumbColor = VigiaColors.Background, checkedTrackColor = VigiaColors.RadarCyan),
        )
    }
}
