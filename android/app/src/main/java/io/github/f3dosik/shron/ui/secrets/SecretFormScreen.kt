package io.github.f3dosik.shron.ui.secrets

import android.text.format.Formatter
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuAnchorType
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.mobile.Mobile
import io.github.f3dosik.shron.ui.common.ErrorMessage
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.f3dosik.shron.ui.common.FormScreen
import io.github.f3dosik.shron.ui.common.GeneratorPanel
import io.github.f3dosik.shron.ui.common.PasswordInput
import io.github.f3dosik.shron.ui.common.PrimaryButton
import io.github.f3dosik.shron.ui.common.StrengthMessage
import io.github.f3dosik.shron.ui.common.TextInput
import io.github.f3dosik.shron.ui.common.rememberExternalActivity

private val types = listOf(
    Mobile.TypeCredentials to R.string.type_credentials_short,
    Mobile.TypeText to R.string.type_text_short,
    Mobile.TypeCard to R.string.type_card_short,
    Mobile.TypeBinary to R.string.type_binary_short,
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SecretFormScreen(vm: SecretFormViewModel, onSaved: (SecretRef) -> Unit, onClose: () -> Unit) {
    BackHandler(onBack = onClose)
    val enabled = !vm.busy

    val offline = vm.coreState.collectAsStateWithLifecycle().value.offline
    FormScreen(
        title = stringResource(if (vm.editing) R.string.secret_edit_title else R.string.secret_new_title),
        subtitle = if (offline && !vm.localOnly) stringResource(R.string.offline_form_notice) else null,
    ) {
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            types.forEachIndexed { index, (type, label) ->
                SegmentedButton(
                    selected = vm.type == type,
                    onClick = { vm.type = type },
                    shape = SegmentedButtonDefaults.itemShape(index, types.size),
                    enabled = enabled && !vm.editing,
                ) { Text(stringResource(label), maxLines = 1) }
            }
        }

        TextInput(
            value = vm.name,
            onValueChange = { vm.name = it },
            label = stringResource(R.string.secret_name),
            supportingText = stringResource(if (vm.editing) R.string.secret_name_fixed else R.string.secret_name_hint),
            enabled = enabled && !vm.editing,
        )

        when (vm.type) {
            Mobile.TypeCredentials -> CredentialsFields(vm, enabled)
            Mobile.TypeText -> OutlinedTextField(
                value = vm.text,
                onValueChange = { vm.text = it },
                label = { Text(stringResource(R.string.secret_text)) },
                enabled = enabled,
                minLines = 4,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 120.dp),
            )
            Mobile.TypeCard -> CardFields(vm, enabled)
            Mobile.TypeBinary -> FileField(vm, enabled)
        }

        GroupField(vm, enabled)

        TextInput(
            value = vm.metadata,
            onValueChange = { vm.metadata = it },
            label = stringResource(R.string.secret_metadata),
            supportingText = stringResource(R.string.secret_metadata_hint),
            enabled = enabled,
        )

        // Где хранить запись, выбирается при создании; потом — переносом в карточке записи.
        if (!vm.editing && !vm.coreState.collectAsStateWithLifecycle().value.standalone) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.secret_local_only), style = MaterialTheme.typography.bodyLarge)
                    Text(
                        stringResource(R.string.secret_local_only_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(checked = vm.localOnly, onCheckedChange = { vm.localOnly = it }, enabled = enabled)
            }
        }

        ErrorMessage(vm.error)
        PrimaryButton(stringResource(R.string.save), busy = vm.busy) { vm.save(onSaved) }
        TextButton(onClick = onClose, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.cancel))
        }
    }
}

/** Группа: свободный ввод с подсказками из существующих групп. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun GroupField(vm: SecretFormViewModel, enabled: Boolean) {
    var expanded by remember { mutableStateOf(false) }
    val suggestions = vm.groups.filter { it.contains(vm.group.trim(), ignoreCase = true) && it != vm.group }
    ExposedDropdownMenuBox(expanded = expanded && suggestions.isNotEmpty(), onExpandedChange = { expanded = it }) {
        OutlinedTextField(
            value = vm.group,
            onValueChange = {
                vm.group = it
                expanded = true
            },
            label = { Text(stringResource(R.string.secret_group)) },
            supportingText = { Text(stringResource(R.string.secret_group_hint)) },
            trailingIcon = if (vm.groups.isNotEmpty()) {
                { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded && suggestions.isNotEmpty()) }
            } else null,
            enabled = enabled,
            singleLine = true,
            keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences, imeAction = ImeAction.Next),
            modifier = Modifier
                .fillMaxWidth()
                .menuAnchor(ExposedDropdownMenuAnchorType.PrimaryEditable, enabled),
        )
        ExposedDropdownMenu(expanded = expanded && suggestions.isNotEmpty(), onDismissRequest = { expanded = false }) {
            suggestions.forEach { group ->
                DropdownMenuItem(
                    text = { Text(group) },
                    onClick = {
                        vm.group = group
                        expanded = false
                    },
                    contentPadding = ExposedDropdownMenuDefaults.ItemContentPadding,
                )
            }
        }
    }
}

@Composable
private fun CredentialsFields(vm: SecretFormViewModel, enabled: Boolean) {
    TextInput(
        value = vm.login,
        onValueChange = { vm.login = it },
        label = stringResource(R.string.secret_login),
        enabled = enabled,
    )
    PasswordInput(
        value = vm.password,
        onValueChange = vm::updatePassword,
        label = stringResource(R.string.secret_password),
        enabled = enabled,
        revealed = vm.generated != null,
    )
    OutlinedButton(onClick = vm::generatePassword, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(R.string.generate))
    }
    StrengthMessage(vm.generated)
    // Настройки появляются после первой генерации: менять длину и наборы — уже для пароля на экране.
    if (vm.generated != null) {
        GeneratorPanel(vm.generator, vm::updateGenerator, onCommit = vm::generatePassword, enabled = enabled)
    }
    OutlinedTextField(
        value = vm.uris,
        onValueChange = { vm.uris = it },
        label = { Text(stringResource(R.string.secret_uris)) },
        placeholder = { Text("github.com") },
        supportingText = { Text(stringResource(R.string.secret_uris_hint)) },
        enabled = enabled,
        minLines = 2,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, autoCorrectEnabled = false),
        modifier = Modifier.fillMaxWidth(),
    )
}

@Composable
private fun CardFields(vm: SecretFormViewModel, enabled: Boolean) {
    TextInput(
        value = vm.cardNumber,
        onValueChange = { vm.cardNumber = it.filter { c -> c.isDigit() || c == ' ' } },
        label = stringResource(R.string.card_number),
        keyboardType = KeyboardType.Number,
        enabled = enabled,
    )
    OutlinedTextField(
        value = vm.cardHolder,
        onValueChange = { vm.cardHolder = it },
        label = { Text(stringResource(R.string.card_holder)) },
        singleLine = true,
        enabled = enabled,
        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters, autoCorrectEnabled = false),
        modifier = Modifier.fillMaxWidth(),
    )
    TextInput(
        value = vm.cardExpiry,
        onValueChange = { vm.cardExpiry = it.filter { c -> c.isDigit() || c == '/' }.take(5) },
        label = stringResource(R.string.card_expiry),
        keyboardType = KeyboardType.Number,
        enabled = enabled,
    )
    PasswordInput(
        value = vm.cardCvv,
        onValueChange = { vm.cardCvv = it.filter(Char::isDigit).take(4) },
        label = stringResource(R.string.card_cvv),
        enabled = enabled,
    )
}

@Composable
private fun FileField(vm: SecretFormViewModel, enabled: Boolean) {
    val context = LocalContext.current
    val pick = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) vm.pickFile(context.contentResolver, uri)
    }
    val pauseLock = rememberExternalActivity()
    val file = vm.file
    val existing = vm.existing
    Text(
        if (file == null && existing != null) {
            stringResource(R.string.file_current, Formatter.formatShortFileSize(context, existing.fileSize))
        } else if (file == null) {
            stringResource(R.string.file_none)
        } else {
            stringResource(R.string.file_selected, file.name, Formatter.formatShortFileSize(context, file.data.size.toLong()))
        },
        style = MaterialTheme.typography.bodyMedium,
    )
    OutlinedButton(onClick = { pauseLock(); pick.launch(arrayOf("*/*")) }, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(R.string.file_pick))
    }
}
