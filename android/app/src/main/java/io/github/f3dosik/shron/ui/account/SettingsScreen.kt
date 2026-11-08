package io.github.f3dosik.shron.ui.account

import androidx.lifecycle.compose.collectAsStateWithLifecycle
import android.os.Build
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ui.common.ErrorMessage
import io.github.f3dosik.shron.ui.common.GeneratorPanel
import io.github.f3dosik.shron.ui.common.PasswordInput
import io.github.f3dosik.shron.ui.common.PrimaryButton
import io.github.f3dosik.shron.ui.common.StrengthMessage
import io.github.f3dosik.shron.ui.common.TextInput
import io.github.f3dosik.shron.ui.common.rememberBiometricPrompter
import io.github.f3dosik.shron.ui.common.rememberExternalActivity
import io.github.f3dosik.shron.ui.server.AutofillSection

@OptIn(ExperimentalMaterial3Api::class)
@Composable
/**
 * Настройки открытого хранилища: аккаунт, безопасность, автозаполнение. Подключение к
 * серверу — отдельным экраном: его сохранение блокирует хранилище.
 */
fun SettingsScreen(
    vm: AccountViewModel,
    login: String,
    serverAddress: String,
    onBack: () -> Unit,
    onConnection: () -> Unit,
) {
    BackHandler(onBack = onBack)
    val snackbar = remember { SnackbarHostState() }
    LaunchedEffect(vm.notice) {
        vm.notice?.let {
            snackbar.showSnackbar(it)
            vm.noticeShown()
        }
    }
    var confirmLogoutAll by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    val enabled = !vm.busy

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.settings_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack, enabled = enabled) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            SectionTitle(stringResource(R.string.account_section))
            ListItem(
                headlineContent = { Text(login) },
                supportingContent = { Text(stringResource(R.string.server_label, serverAddress)) },
                leadingContent = { Icon(Icons.Filled.AccountCircle, contentDescription = null) },
            )

            HorizontalDivider()
            SectionTitle(stringResource(R.string.security_section))
            BiometricSection(vm, enabled)
            ListItem(
                headlineContent = { Text(stringResource(R.string.connection_open)) },
                supportingContent = { Text(stringResource(R.string.connection_open_hint)) },
                trailingContent = { Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null) },
                modifier = Modifier.clickable(enabled = enabled, onClick = onConnection),
            )

            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                HorizontalDivider()
                AutofillSection(onExternalActivity = rememberExternalActivity())
            }

            HorizontalDivider()
            ChangePasswordSection(vm, enabled)

            HorizontalDivider()
            Text(stringResource(R.string.sessions_section), style = MaterialTheme.typography.titleMedium)
            Text(
                stringResource(R.string.logout_everywhere_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            OutlinedButton(onClick = { confirmLogoutAll = true }, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.logout_everywhere))
            }

            HorizontalDivider()
            Text(stringResource(R.string.delete_account), style = MaterialTheme.typography.titleMedium)
            Text(
                stringResource(R.string.delete_account_text, login),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            OutlinedButton(
                onClick = {
                    vm.resetDeleteForm()
                    confirmDelete = true
                },
                enabled = enabled,
                colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error),
                modifier = Modifier.fillMaxWidth(),
            ) { Text(stringResource(R.string.delete_account)) }
        }
    }

    if (confirmLogoutAll) {
        AlertDialog(
            onDismissRequest = { confirmLogoutAll = false },
            title = { Text(stringResource(R.string.logout_everywhere)) },
            text = {
                val localRecords = vm.coreState.collectAsStateWithLifecycle().value.localRecords
                Text(
                    stringResource(R.string.logout_everywhere_confirm) +
                        if (localRecords > 0) "\n\n" + stringResource(R.string.logout_local_text, localRecords) else "",
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    confirmLogoutAll = false
                    vm.logoutEverywhere()
                }) { Text(stringResource(R.string.logout_everywhere_short)) }
            },
            dismissButton = {
                TextButton(onClick = { confirmLogoutAll = false }) { Text(stringResource(R.string.cancel)) }
            },
        )
    }

    if (confirmDelete) {
        DeleteAccountDialog(vm, login, onDismiss = {
            confirmDelete = false
            vm.resetDeleteForm()
        })
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(text, style = MaterialTheme.typography.titleMedium)
}

@Composable
private fun BiometricSection(vm: AccountViewModel, enabled: Boolean) {
    val prompter = rememberBiometricPrompter()
    val pauseLock = rememberExternalActivity()
    val title = stringResource(R.string.biometric_enable_title)
    val available = vm.biometricAvailable

    Text(stringResource(R.string.biometric_section), style = MaterialTheme.typography.titleSmall)
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(stringResource(R.string.biometric_switch), modifier = Modifier.weight(1f))
        Switch(
            checked = vm.biometricEnabled,
            onCheckedChange = { on ->
                if (!on) {
                    vm.disableBiometric()
                } else {
                    vm.biometricEnrollCipher()?.let { cipher ->
                        // Запрос биометрии может увести приложение в фон на некоторых телефонах.
                        pauseLock()
                        prompter.authenticate(title, null, cipher, onSuccess = vm::enableBiometric, onError = vm::biometricError)
                    }
                }
            },
            enabled = enabled && (available || vm.biometricEnabled),
        )
    }
    Text(
        stringResource(if (available || vm.biometricEnabled) R.string.biometric_hint else R.string.biometric_unavailable),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun ChangePasswordSection(vm: AccountViewModel, enabled: Boolean) {
    var advanced by remember { mutableStateOf(false) }

    Text(stringResource(R.string.change_password), style = MaterialTheme.typography.titleMedium)
    PasswordInput(
        value = vm.currentPassword,
        onValueChange = { vm.currentPassword = it },
        label = stringResource(R.string.current_master_password),
        enabled = enabled,
    )
    PasswordInput(
        value = vm.newPassword,
        onValueChange = vm::updateNewPassword,
        label = stringResource(R.string.new_master_password),
        enabled = enabled,
        revealed = vm.generated != null,
    )
    PasswordInput(
        value = vm.confirm,
        onValueChange = vm::updateConfirm,
        label = stringResource(R.string.new_master_password_repeat),
        enabled = enabled,
        revealed = vm.generated != null,
    )
    OutlinedButton(onClick = vm::generatePassword, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(R.string.generate_master_password))
    }
    StrengthMessage(vm.generated)
    if (vm.generated != null) {
        GeneratorPanel(vm.generator, vm::updateGenerator, onCommit = vm::generatePassword, enabled = enabled)
    }

    TextButton(onClick = { advanced = !advanced }) {
        Text(stringResource(if (advanced) R.string.kdf_hide else R.string.kdf_show))
    }
    if (advanced) {
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            TextInput(
                value = vm.kdfTime,
                onValueChange = { vm.kdfTime = it.filter(Char::isDigit).take(2) },
                label = stringResource(R.string.kdf_time),
                keyboardType = KeyboardType.Number,
                enabled = enabled,
                modifier = Modifier.weight(1f),
            )
            TextInput(
                value = vm.kdfMemoryMiB,
                onValueChange = { vm.kdfMemoryMiB = it.filter(Char::isDigit).take(4) },
                label = stringResource(R.string.kdf_memory),
                keyboardType = KeyboardType.Number,
                enabled = enabled,
                modifier = Modifier.weight(1f),
            )
        }
        Text(
            stringResource(R.string.kdf_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    Text(
        stringResource(R.string.change_password_hint),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    ErrorMessage(vm.error)
    PrimaryButton(
        stringResource(if (vm.busy) R.string.change_password_busy else R.string.change_password_button),
        busy = vm.busy,
        enabled = vm.currentPassword.isNotEmpty() && vm.newPassword.isNotEmpty(),
        onClick = vm::changePassword,
    )
}

/** Удаление учётки: как на десктопе, нужно ввести логин и мастер-пароль. */
@Composable
private fun DeleteAccountDialog(vm: AccountViewModel, login: String, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = { if (!vm.busy) onDismiss() },
        title = { Text(stringResource(R.string.delete_account)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.delete_account_text, login))
                TextInput(
                    value = vm.deleteLogin,
                    onValueChange = { vm.deleteLogin = it },
                    label = stringResource(R.string.delete_account_type_login),
                    enabled = !vm.busy,
                )
                PasswordInput(
                    value = vm.deletePassword,
                    onValueChange = { vm.deletePassword = it },
                    label = stringResource(R.string.master_password),
                    enabled = !vm.busy,
                )
                ErrorMessage(vm.error)
            }
        },
        confirmButton = {
            TextButton(
                onClick = { vm.deleteAccount(login) },
                enabled = !vm.busy && vm.deleteLogin.trim() == login && vm.deletePassword.isNotEmpty(),
            ) { Text(stringResource(R.string.delete_account_confirm), color = MaterialTheme.colorScheme.error) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !vm.busy) { Text(stringResource(R.string.cancel)) }
        },
    )
}
