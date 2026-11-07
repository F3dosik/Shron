package io.github.f3dosik.shron.ui.auth

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.core.BackgroundLock
import io.github.f3dosik.shron.mobile.Mobile
import io.github.f3dosik.shron.ui.common.ErrorMessage
import io.github.f3dosik.shron.ui.common.FormScreen
import io.github.f3dosik.shron.ui.common.GeneratorPanel
import io.github.f3dosik.shron.ui.common.InfoMessage
import io.github.f3dosik.shron.ui.common.PasswordInput
import io.github.f3dosik.shron.ui.common.PrimaryButton
import io.github.f3dosik.shron.ui.common.StrengthMessage
import io.github.f3dosik.shron.ui.common.TextInput
import io.github.f3dosik.shron.ui.common.rememberBiometricPrompter

@Composable
fun SignInScreen(vm: AuthViewModel, serverAddress: String, onServerSettings: () -> Unit) {
    FormScreen(
        title = stringResource(if (vm.registering) R.string.register_title else R.string.sign_in_title),
        subtitle = stringResource(R.string.server_label, serverAddress),
    ) {
        TextInput(
            value = vm.login,
            onValueChange = { vm.login = it },
            label = stringResource(R.string.login),
            enabled = !vm.busy,
        )
        PasswordInput(
            value = vm.password,
            onValueChange = vm::updatePassword,
            label = stringResource(R.string.master_password),
            enabled = !vm.busy,
            revealed = vm.generated != null,
            onDone = if (vm.registering) null else vm::submitSignIn,
        )
        if (vm.registering) {
            PasswordInput(
                value = vm.confirm,
                onValueChange = vm::updateConfirm,
                label = stringResource(R.string.master_password_repeat),
                enabled = !vm.busy,
                revealed = vm.generated != null,
                onDone = vm::submitSignIn,
            )
            OutlinedButton(onClick = vm::generateMasterPassword, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.generate_master_password))
            }
            StrengthMessage(vm.generated)
            if (vm.generated != null) {
                GeneratorPanel(vm.generator, vm::updateGenerator, onCommit = vm::generateMasterPassword, enabled = !vm.busy)
            }
            Text(
                stringResource(
                    if (vm.generated != null) R.string.generated_master_password_hint else R.string.register_warning,
                ),
                style = MaterialTheme.typography.bodyMedium,
            )
        }

        InfoMessage(vm.info)
        ErrorMessage(vm.error)

        PrimaryButton(
            stringResource(if (vm.registering) R.string.register else R.string.sign_in),
            busy = vm.busy,
            onClick = vm::submitSignIn,
        )
        TextButton(onClick = vm::toggleRegistering, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(if (vm.registering) R.string.switch_to_sign_in else R.string.switch_to_register))
        }
        TextButton(onClick = onServerSettings, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.server_settings))
        }
    }
}

@Composable
fun UnlockScreen(vm: AuthViewModel, login: String, lockReason: String?, onServerSettings: () -> Unit) {
    val reason = when (lockReason) {
        Mobile.LockIdle -> stringResource(R.string.locked_idle)
        Mobile.LockSessionExpired -> stringResource(R.string.locked_session_expired)
        BackgroundLock.LOCK_REASON -> stringResource(R.string.locked_background)
        else -> null
    }
    FormScreen(
        title = stringResource(R.string.unlock_title),
        subtitle = listOfNotNull(reason, stringResource(R.string.account_label, login)).joinToString(" "),
    ) {
        PasswordInput(
            value = vm.password,
            onValueChange = vm::updatePassword,
            label = stringResource(R.string.master_password),
            enabled = !vm.busy,
            onDone = vm::unlock,
        )
        ErrorMessage(vm.error)
        PrimaryButton(stringResource(R.string.unlock), busy = vm.busy, onClick = vm::unlock)
        // После ручной блокировки запрос сразу не показывается: пользователь только что
        // сам закрыл хранилище.
        BiometricUnlockButton(vm, autoPrompt = lockReason != Mobile.LockManual)
        TextButton(onClick = vm::logout, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.logout))
        }
        TextButton(onClick = onServerSettings, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.server_settings))
        }
    }
}

@Composable
fun PasswordChangeScreen(vm: AuthViewModel, login: String) {
    FormScreen(
        title = stringResource(R.string.password_change_title),
        subtitle = stringResource(R.string.password_change_subtitle, login),
    ) {
        PasswordInput(
            value = vm.newPassword,
            onValueChange = { vm.newPassword = it },
            label = stringResource(R.string.new_master_password),
            enabled = !vm.busy,
        )
        PasswordInput(
            value = vm.newConfirm,
            onValueChange = { vm.newConfirm = it },
            label = stringResource(R.string.new_master_password_repeat),
            enabled = !vm.busy,
            onDone = vm::completePasswordChange,
        )
        ErrorMessage(vm.error)
        PrimaryButton(stringResource(R.string.password_change_save), busy = vm.busy, onClick = vm::completePasswordChange)
        TextButton(onClick = vm::cancelPasswordChange, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.cancel))
        }
    }
}

/** Предупреждение о понижении параметров Argon2id: принимать только если пользователь сам их сменил. */
@Composable
fun KdfDowngradeDialog(vm: AuthViewModel) {
    val warning = vm.kdfDowngrade ?: return
    AlertDialog(
        onDismissRequest = vm::rejectKdfChange,
        title = { Text(stringResource(R.string.kdf_downgrade_title)) },
        text = { Text(warning.message) },
        confirmButton = {
            TextButton(onClick = vm::acceptKdfChange) { Text(stringResource(R.string.kdf_downgrade_accept)) }
        },
        dismissButton = {
            TextButton(onClick = vm::rejectKdfChange) { Text(stringResource(R.string.cancel)) }
        },
    )
}

/**
 * Разблокировка по биометрии, если она включена для учётки. [autoPrompt] — показать
 * запрос сразу при открытии экрана, не дожидаясь нажатия.
 */
@Composable
fun BiometricUnlockButton(vm: AuthViewModel, autoPrompt: Boolean) {
    val prompter = rememberBiometricPrompter()
    val title = stringResource(R.string.biometric_unlock_title)
    val prompt = {
        vm.biometricCipher()?.let { cipher ->
            prompter.authenticate(title, null, cipher, onSuccess = vm::unlockWithBiometric, onError = vm::biometricError)
        }
    }
    LaunchedEffect(Unit) {
        vm.refreshBiometric()
        if (autoPrompt && vm.biometricReady) prompt()
    }
    if (vm.biometricReady) {
        OutlinedButton(onClick = { prompt() }, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.biometric_unlock))
        }
    }
}
