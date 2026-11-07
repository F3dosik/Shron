package io.github.f3dosik.shron.autofill

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.ui.auth.AuthViewModel
import io.github.f3dosik.shron.ui.auth.BiometricUnlockButton
import io.github.f3dosik.shron.ui.auth.KdfDowngradeDialog
import io.github.f3dosik.shron.ui.common.ErrorMessage
import io.github.f3dosik.shron.ui.common.FormScreen
import io.github.f3dosik.shron.ui.common.PasswordInput
import io.github.f3dosik.shron.ui.common.PrimaryButton
import io.github.f3dosik.shron.ui.secrets.SecretDetails

/**
 * Экран окна выбора. Учётка и сервер настраиваются только в приложении; здесь — лишь
 * разблокировка и выбор логина.
 */
@Composable
fun AutofillPicker(
    app: ShronApplication,
    requester: String,
    keyword: String,
    onFill: (SecretDetails) -> Unit,
    onOpenApp: () -> Unit,
    onCancel: () -> Unit,
) {
    BackHandler(onBack = onCancel)
    val state by app.core.state.collectAsStateWithLifecycle()
    val auth: AuthViewModel = viewModel { AuthViewModel(app) }

    when {
        // Временный пароль меняется в приложении: там экран смены с подтверждением.
        state.login.isEmpty() || auth.temporaryPassword != null -> FormScreen(
            title = stringResource(R.string.autofill_title),
            subtitle = stringResource(R.string.autofill_sign_in_first),
        ) {
            PrimaryButton(stringResource(R.string.autofill_open_app), busy = false, onClick = onOpenApp)
            TextButton(onClick = onCancel, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.cancel)) }
        }

        !state.unlocked -> FormScreen(
            title = stringResource(R.string.autofill_title),
            subtitle = stringResource(R.string.autofill_unlock, requester, state.login),
        ) {
            PasswordInput(
                value = auth.password,
                onValueChange = auth::updatePassword,
                label = stringResource(R.string.master_password),
                enabled = !auth.busy,
                onDone = auth::unlock,
            )
            ErrorMessage(auth.error)
            PrimaryButton(stringResource(R.string.unlock), busy = auth.busy, onClick = auth::unlock)
            BiometricUnlockButton(auth, autoPrompt = true)
            TextButton(onClick = onCancel, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.cancel)) }
        }

        else -> {
            val vm: AutofillPickerViewModel = viewModel { AutofillPickerViewModel(app, keyword) }
            LaunchedEffect(Unit) { vm.load() }
            CredentialList(vm, requester, onFill, onCancel)
        }
    }
    KdfDowngradeDialog(auth)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun CredentialList(
    vm: AutofillPickerViewModel,
    requester: String,
    onFill: (SecretDetails) -> Unit,
    onCancel: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(stringResource(R.string.autofill_fill_for, requester), maxLines = 1)
                },
                navigationIcon = {
                    IconButton(onClick = onCancel) {
                        Icon(Icons.Filled.Close, contentDescription = stringResource(R.string.cancel))
                    }
                },
            )
        },
    ) { padding ->
        LazyColumn(
            Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            item {
                OutlinedTextField(
                    value = vm.query,
                    onValueChange = { vm.query = it },
                    placeholder = { Text(stringResource(R.string.search)) },
                    leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
                    trailingIcon = if (vm.query.isNotEmpty()) {
                        {
                            IconButton(onClick = { vm.query = "" }) {
                                Icon(Icons.Filled.Clear, stringResource(R.string.search_clear))
                            }
                        }
                    } else null,
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(autoCorrectEnabled = false, imeAction = ImeAction.Search),
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 8.dp),
                )
                if (vm.busy) LinearProgressIndicator(Modifier.fillMaxWidth())
                vm.error?.let {
                    ErrorMessageRow(it.message)
                }
            }
            val items = vm.items
            if (items != null && items.isEmpty()) {
                item {
                    Text(
                        stringResource(if (vm.query.isBlank()) R.string.autofill_no_logins else R.string.autofill_nothing_found),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(24.dp),
                    )
                }
            }
            items(items.orEmpty(), key = { it.name }) { item ->
                ListItem(
                    headlineContent = { Text(item.name) },
                    supportingContent = item.metadata.takeIf { it.isNotBlank() }?.let { { Text(it, maxLines = 1) } },
                    modifier = Modifier.clickable(enabled = !vm.busy) { vm.select(item, onFill) },
                )
                HorizontalDivider()
            }
        }
    }
}

@Composable
private fun ErrorMessageRow(message: String) {
    Text(message, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(horizontal = 16.dp))
}
