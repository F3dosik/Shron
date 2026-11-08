package io.github.f3dosik.shron.ui.server

import androidx.lifecycle.compose.collectAsStateWithLifecycle
import android.content.Intent
import android.os.Build
import android.provider.Settings
import android.view.autofill.AutofillManager
import androidx.annotation.RequiresApi
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.core.net.toUri
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.core.BackgroundLock
import io.github.f3dosik.shron.ui.common.ErrorMessage
import io.github.f3dosik.shron.ui.common.FormScreen
import io.github.f3dosik.shron.ui.common.InfoMessage
import io.github.f3dosik.shron.ui.common.PrimaryButton
import io.github.f3dosik.shron.ui.common.TextInput
import io.github.f3dosik.shron.ui.common.rememberExternalActivity

/** Настройки сервера. [onClose] — null при первом запуске, когда возвращаться некуда. */
@Composable
fun ServerScreen(vm: ServerViewModel, onSaved: () -> Unit, onClose: (() -> Unit)?) {
    val context = LocalContext.current
    val pickCA = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) vm.loadCA(context.contentResolver, uri)
    }
    val pauseLock = rememberExternalActivity()
    if (onClose != null) BackHandler(onBack = onClose)

    val coreState by vm.coreState.collectAsStateWithLifecycle()
    FormScreen(
        title = stringResource(R.string.server_title),
        subtitle = stringResource(if (vm.standalone) R.string.standalone_mode_subtitle else R.string.server_subtitle),
    ) {
        if (vm.canChooseMode) ModeChoice(vm.standalone, onChange = { vm.standalone = it }, enabled = !vm.busy)
        if (vm.standalone) {
            Text(
                stringResource(R.string.standalone_mode_hint),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            ServerFields(vm, onPickCA = { pauseLock(); pickCA.launch(arrayOf("*/*")) })
        }

        SectionTitle(stringResource(R.string.lock_section))
        TextInput(
            value = vm.autoLockMinutes,
            onValueChange = { vm.autoLockMinutes = it.filter(Char::isDigit) },
            label = stringResource(R.string.server_auto_lock),
            supportingText = stringResource(R.string.server_auto_lock_hint),
            keyboardType = KeyboardType.Number,
            enabled = !vm.busy,
        )

        BackgroundLockChoice(vm.backgroundLockSeconds, { vm.backgroundLockSeconds = it }, enabled = !vm.busy)

        // Без сервера все записи на телефоне, и при подключении сервера они не удаляются.
        if (!coreState.standalone && coreState.localRecords > 0) {
            Text(
                stringResource(R.string.server_local_warning, coreState.localRecords),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
            )
        }
        if (coreState.standalone && !vm.standalone && coreState.hasStandaloneVault) {
            Text(
                stringResource(R.string.standalone_connect_hint),
                style = MaterialTheme.typography.bodyMedium,
            )
        }
        ErrorMessage(vm.error)
        InfoMessage(vm.checkResult)

        PrimaryButton(stringResource(R.string.save), busy = vm.busy) { vm.save(onSaved) }
        if (!vm.standalone) {
            OutlinedButton(onClick = vm::check, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.server_check))
            }
        }
        if (onClose != null) {
            TextButton(onClick = onClose, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.cancel)) }
        }
    }
}

/** Выбор: хранить записи на сервере или только на телефоне. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ModeChoice(standalone: Boolean, onChange: (Boolean) -> Unit, enabled: Boolean) {
    val options = listOf(false to R.string.mode_server, true to R.string.mode_standalone)
    SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
        options.forEachIndexed { index, (value, label) ->
            SegmentedButton(
                selected = standalone == value,
                onClick = { onChange(value) },
                shape = SegmentedButtonDefaults.itemShape(index, options.size),
                enabled = enabled,
            ) { Text(stringResource(label), maxLines = 1) }
        }
    }
}

/** Адрес сервера и корневой сертификат. */
@Composable
private fun ServerFields(vm: ServerViewModel, onPickCA: () -> Unit) {
    SectionTitle(stringResource(R.string.server_section))
    TextInput(
        value = vm.address,
        onValueChange = { vm.address = it },
        label = stringResource(R.string.server_address),
        supportingText = stringResource(R.string.server_address_hint),
        keyboardType = KeyboardType.Uri,
        enabled = !vm.busy,
    )

    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(stringResource(R.string.server_ca), style = MaterialTheme.typography.titleSmall)
        Text(
            when (val ca = vm.ca) {
                CaSource.System -> stringResource(R.string.server_ca_system)
                CaSource.Saved -> stringResource(R.string.server_ca_saved)
                is CaSource.File -> stringResource(R.string.server_ca_file, ca.name)
            },
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = onPickCA, enabled = !vm.busy) {
                Text(stringResource(R.string.server_ca_pick))
            }
            if (vm.ca != CaSource.System) {
                TextButton(onClick = vm::useSystemCA, enabled = !vm.busy) {
                    Text(stringResource(R.string.server_ca_use_system))
                }
            }
        }
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(text, style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 8.dp))
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun BackgroundLockChoice(seconds: Int, onChange: (Int) -> Unit, enabled: Boolean) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(stringResource(R.string.background_lock), style = MaterialTheme.typography.titleSmall)
        Text(
            stringResource(R.string.background_lock_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        val options = BackgroundLock.DELAY_OPTIONS_SECONDS
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            options.forEachIndexed { index, option ->
                SegmentedButton(
                    selected = seconds == option,
                    onClick = { onChange(option) },
                    shape = SegmentedButtonDefaults.itemShape(index, options.size),
                    enabled = enabled,
                ) {
                    Text(
                        when {
                            option == 0 -> stringResource(R.string.background_lock_now)
                            option < 60 -> stringResource(R.string.background_lock_seconds, option)
                            else -> stringResource(R.string.background_lock_minutes, option / 60)
                        },
                        maxLines = 1,
                    )
                }
            }
        }
    }
}

/** Служба автозаполнения выбирается в системных настройках; здесь — состояние и переход туда. */
@RequiresApi(Build.VERSION_CODES.O)
@Composable
fun AutofillSection(onExternalActivity: () -> Unit) {
    val context = LocalContext.current
    val manager = remember(context) { context.getSystemService(AutofillManager::class.java) }
    var enabled by remember { mutableStateOf(manager?.hasEnabledAutofillServices() == true) }
    // После возвращения из системных настроек состояние могло измениться.
    LifecycleResumeEffect(manager) {
        enabled = manager?.hasEnabledAutofillServices() == true
        onPauseOrDispose {}
    }

    SectionTitle(stringResource(R.string.autofill_section))
    Text(
        stringResource(R.string.autofill_hint),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    if (enabled) {
        Text(stringResource(R.string.autofill_enabled), style = MaterialTheme.typography.bodyMedium)
    } else if (manager?.isAutofillSupported == true) {
        OutlinedButton(
            onClick = {
                onExternalActivity()
                context.startActivity(
                    Intent(Settings.ACTION_REQUEST_SET_AUTOFILL_SERVICE, "package:${context.packageName}".toUri()),
                )
            },
            modifier = Modifier.fillMaxWidth(),
        ) { Text(stringResource(R.string.autofill_enable)) }
    }
}
