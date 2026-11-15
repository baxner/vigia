package com.baxner.vigia.ui.agent

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.baxner.vigia.R
import com.baxner.vigia.data.agent.DiscoveredAgent
import com.baxner.vigia.ui.components.GlassSurface
import com.baxner.vigia.ui.components.SectionCard
import com.baxner.vigia.ui.components.VigiaTextField
import com.baxner.vigia.ui.theme.VigiaColors
import com.baxner.vigia.ui.theme.VigiaMono

/** Envoltura con el [AgentPairingViewModel] real; ver [AgentPairingScreenContent] para la versión sin estado (capturas, tests). */
@Composable
fun AgentPairingScreen(onDone: () -> Unit, onCancel: () -> Unit, modifier: Modifier = Modifier) {
    val viewModel: AgentPairingViewModel = viewModel()
    val step by viewModel.step.collectAsStateWithLifecycle()

    LaunchedEffect(step) {
        if (step is PairingStep.Done) onDone()
    }

    AgentPairingScreenContent(
        step = step,
        onBack = onCancel,
        onRescan = viewModel::startDiscovery,
        onSelectAgent = viewModel::selectDiscovered,
        onToggleManualEntry = viewModel::toggleManualEntry,
        onManualHostChange = viewModel::onManualHostChange,
        onManualPortChange = viewModel::onManualPortChange,
        onContinueManual = viewModel::continueManual,
        onCodeChange = viewModel::onCodeChange,
        onSubmitCode = viewModel::submitCode,
        modifier = modifier,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AgentPairingScreenContent(
    step: PairingStep,
    onBack: () -> Unit,
    onRescan: () -> Unit,
    onSelectAgent: (DiscoveredAgent) -> Unit,
    onToggleManualEntry: () -> Unit,
    onManualHostChange: (String) -> Unit,
    onManualPortChange: (String) -> Unit,
    onContinueManual: () -> Unit,
    onCodeChange: (String) -> Unit,
    onSubmitCode: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Scaffold(
        modifier = modifier,
        containerColor = VigiaColors.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringRes(R.string.pairing_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringRes(R.string.a11y_back), tint = VigiaColors.TextPrimary)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = VigiaColors.Background, titleContentColor = VigiaColors.TextPrimary),
            )
        },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when (step) {
                is PairingStep.ChooseAgent -> ChooseAgentContent(
                    step = step,
                    onRescan = onRescan,
                    onSelectAgent = onSelectAgent,
                    onToggleManualEntry = onToggleManualEntry,
                    onManualHostChange = onManualHostChange,
                    onManualPortChange = onManualPortChange,
                    onContinueManual = onContinueManual,
                )
                is PairingStep.EnterCode -> EnterCodeContent(step = step, onCodeChange = onCodeChange, onSubmit = onSubmitCode)
                is PairingStep.Done -> Unit // VigiaApp navega fuera apenas llega (ver AgentPairingScreen.onDone).
            }
        }
    }
}

@Composable
private fun ChooseAgentContent(
    step: PairingStep.ChooseAgent,
    onRescan: () -> Unit,
    onSelectAgent: (DiscoveredAgent) -> Unit,
    onToggleManualEntry: () -> Unit,
    onManualHostChange: (String) -> Unit,
    onManualPortChange: (String) -> Unit,
    onContinueManual: () -> Unit,
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (step.searching) {
            item(key = "searching") {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = VigiaColors.RadarCyan, trackColor = VigiaColors.Outline)
                    Text(stringRes(R.string.pairing_searching), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
                }
            }
        }

        if (step.discovered.isNotEmpty()) {
            items(step.discovered, key = { it.id }) { agent ->
                DiscoveredAgentRow(agent, onClick = { onSelectAgent(agent) })
            }
        } else if (!step.searching) {
            item(key = "none") {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(stringRes(R.string.pairing_found_none), style = MaterialTheme.typography.bodyMedium, color = VigiaColors.TextSecondary)
                    TextButton(onClick = onRescan) { Text(stringRes(R.string.pairing_rescan), color = VigiaColors.RadarCyan) }
                }
            }
        }

        item(key = "manual_toggle") {
            TextButton(onClick = onToggleManualEntry) {
                Text(stringRes(R.string.pairing_manual_toggle), color = VigiaColors.RadarCyan)
            }
        }

        if (step.showManualEntry) {
            item(key = "manual_entry") {
                SectionCard(title = stringRes(R.string.pairing_manual_toggle)) {
                    VigiaTextField(
                        value = step.manualHost,
                        onValueChange = onManualHostChange,
                        placeholder = stringRes(R.string.pairing_manual_host),
                    )
                    VigiaTextField(
                        value = step.manualPort,
                        onValueChange = onManualPortChange,
                        placeholder = stringRes(R.string.pairing_manual_port),
                    )
                    Button(
                        onClick = onContinueManual,
                        enabled = !step.checkingManual,
                        modifier = Modifier.fillMaxWidth(),
                        colors = ButtonDefaults.buttonColors(containerColor = VigiaColors.RadarCyan, contentColor = VigiaColors.Background),
                    ) {
                        if (step.checkingManual) {
                            CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = VigiaColors.Background)
                        } else {
                            Text(stringRes(R.string.pairing_manual_continue))
                        }
                    }
                }
            }
        }

        step.errorMessage?.let { message ->
            item(key = "error") {
                Text(message, style = MaterialTheme.typography.bodySmall, color = VigiaColors.Risk)
            }
        }
    }
}

@Composable
private fun DiscoveredAgentRow(agent: DiscoveredAgent, onClick: () -> Unit) {
    GlassSurface(modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).semantics(mergeDescendants = true) {}) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(Icons.Default.Dns, contentDescription = null, tint = VigiaColors.RadarCyan)
            Column(modifier = Modifier.weight(1f)) {
                Text(agent.name, style = MaterialTheme.typography.bodyLarge, color = VigiaColors.TextPrimary)
                Text("${agent.host}:${agent.port}", style = VigiaMono.label, color = VigiaColors.TextSecondary)
            }
            Icon(Icons.Default.ChevronRight, contentDescription = null, tint = VigiaColors.TextTertiary)
        }
    }
}

@Composable
private fun EnterCodeContent(step: PairingStep.EnterCode, onCodeChange: (String) -> Unit, onSubmit: () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text(
            stringRes(R.string.pairing_code_target, step.target.name),
            style = MaterialTheme.typography.titleMedium,
            color = VigiaColors.TextPrimary,
            textAlign = TextAlign.Center,
        )
        Text(
            stringRes(R.string.pairing_code_instructions),
            style = VigiaMono.body,
            color = VigiaColors.TextSecondary,
            textAlign = TextAlign.Center,
        )

        GlassSurface(modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(20.dp)) {
            androidx.compose.material3.OutlinedTextField(
                value = step.code,
                onValueChange = onCodeChange,
                modifier = Modifier.fillMaxWidth().padding(8.dp),
                textStyle = BigDigitsStyle,
                singleLine = true,
                placeholder = { Text("······", style = BigDigitsStyle, color = VigiaColors.TextTertiary, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth()) },
                keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.NumberPassword),
                colors = androidx.compose.material3.OutlinedTextFieldDefaults.colors(
                    focusedBorderColor = Color.Transparent,
                    unfocusedBorderColor = Color.Transparent,
                    focusedTextColor = VigiaColors.RadarCyan,
                    unfocusedTextColor = VigiaColors.RadarCyan,
                    cursorColor = VigiaColors.RadarCyan,
                    focusedContainerColor = Color.Transparent,
                    unfocusedContainerColor = Color.Transparent,
                ),
            )
        }

        step.errorMessage?.let { message ->
            Text(message, style = MaterialTheme.typography.bodySmall, color = VigiaColors.Risk, textAlign = TextAlign.Center)
        }

        Button(
            onClick = onSubmit,
            enabled = step.code.length == 6 && !step.submitting,
            modifier = Modifier.fillMaxWidth().height(52.dp),
            colors = ButtonDefaults.buttonColors(
                containerColor = VigiaColors.RadarCyan,
                contentColor = VigiaColors.Background,
                disabledContainerColor = VigiaColors.Outline,
                disabledContentColor = VigiaColors.TextTertiary,
            ),
            shape = MaterialTheme.shapes.large,
        ) {
            if (step.submitting) {
                CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = VigiaColors.Background)
            } else {
                Text(stringRes(R.string.pairing_code_submit), fontWeight = FontWeight.SemiBold)
            }
        }
    }
}

private val BigDigitsStyle = TextStyle(
    fontFamily = VigiaMono.large.fontFamily,
    fontSize = 40.sp,
    letterSpacing = 12.sp,
    textAlign = TextAlign.Center,
)

@Composable
private fun stringRes(id: Int): String = androidx.compose.ui.res.stringResource(id)

@Composable
private fun stringRes(id: Int, arg: String): String = androidx.compose.ui.res.stringResource(id, arg)
