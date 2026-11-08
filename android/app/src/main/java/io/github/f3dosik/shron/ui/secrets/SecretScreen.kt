package io.github.f3dosik.shron.ui.secrets

import android.text.format.Formatter
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.mobile.Mobile
import io.github.f3dosik.shron.ui.common.ErrorMessage
import io.github.f3dosik.shron.ui.common.OfflineNotice
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.f3dosik.shron.ui.common.rememberExternalActivity
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SecretScreen(
    vm: SecretViewModel,
    onBack: () -> Unit,
    onEdit: (SecretDetails) -> Unit,
    onDeleted: () -> Unit,
) {
    BackHandler(onBack = onBack)
    val snackbar = remember { SnackbarHostState() }
    LaunchedEffect(vm.notice) {
        vm.notice?.let {
            snackbar.showSnackbar(it)
            vm.noticeShown()
        }
    }
    var confirmDelete by remember { mutableStateOf(false) }
    var menu by remember { mutableStateOf(false) }
    val secret = vm.secret
    val offline = vm.coreState.collectAsStateWithLifecycle().value.offline

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(vm.ref.name, maxLines = 1) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
                actions = {
                    if (secret != null) {
                        IconButton(onClick = { onEdit(secret) }, enabled = !vm.busy) {
                            Icon(Icons.Filled.Edit, contentDescription = stringResource(R.string.edit))
                        }
                        // Удаление — в меню, а не рядом с «Изменить»: так его не задеть случайно.
                        Box {
                            IconButton(onClick = { menu = true }, enabled = !vm.busy) {
                                Icon(Icons.Filled.MoreVert, contentDescription = stringResource(R.string.secret_menu))
                            }
                            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                                DropdownMenuItem(
                                    text = { Text(stringResource(R.string.delete), color = MaterialTheme.colorScheme.error) },
                                    leadingIcon = {
                                        Icon(Icons.Filled.Delete, contentDescription = null, tint = MaterialTheme.colorScheme.error)
                                    },
                                    onClick = {
                                        menu = false
                                        confirmDelete = true
                                    },
                                )
                            }
                        }
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            if (offline) OfflineNotice(stringResource(R.string.offline_secret_notice))
            Column(
                Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 16.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                ErrorMessage(vm.error)
                when {
                    secret != null -> SecretFields(vm, secret)
                    vm.busy -> CircularProgressIndicator(Modifier.align(Alignment.CenterHorizontally))
                    vm.error != null -> TextButton(onClick = vm::load) { Text(stringResource(R.string.retry)) }
                }
            }
        }
    }

    if (confirmDelete) {
        AlertDialog(
            onDismissRequest = { confirmDelete = false },
            title = { Text(stringResource(R.string.delete_secret_title)) },
            text = { Text(stringResource(R.string.delete_secret_text, vm.ref.name)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmDelete = false
                    vm.delete(onDeleted)
                }) { Text(stringResource(R.string.delete), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = {
                TextButton(onClick = { confirmDelete = false }) { Text(stringResource(R.string.cancel)) }
            },
        )
    }
}

@Composable
private fun SecretFields(vm: SecretViewModel, secret: SecretDetails) {
    val context = LocalContext.current
    val pauseLock = rememberExternalActivity()
    val saveFile = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("application/octet-stream"),
    ) { uri -> if (uri != null) vm.saveFile(context.contentResolver, uri) }

    val type = typeLabel(secret.ref.type)?.let { stringResource(it) }
    val heading = listOfNotNull(type, secret.group.ifBlank { null }).joinToString(" · ")
    if (heading.isNotEmpty()) {
        Text(heading, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
    }

    when (secret.ref.type) {
        Mobile.TypeCredentials -> {
            Field(stringResource(R.string.secret_login), secret.login, vm)
            Field(stringResource(R.string.secret_password), secret.password, vm, hidden = true)
            if (secret.uris.isNotBlank()) {
                Text(stringResource(R.string.secret_uris), style = MaterialTheme.typography.labelMedium)
                Text(secret.uris, style = MaterialTheme.typography.bodyMedium)
            }
        }
        Mobile.TypeText -> Field(stringResource(R.string.secret_text), secret.text, vm, multiline = true)
        Mobile.TypeCard -> {
            Field(
                stringResource(R.string.card_number), secret.cardNumber, vm, hidden = true,
                display = formatCardNumber(secret.cardNumber), masked = maskCardNumber(secret.cardNumber),
            )
            Field(stringResource(R.string.card_holder), secret.cardHolder, vm)
            Field(stringResource(R.string.card_expiry_short), secret.cardExpiry, vm)
            Field(stringResource(R.string.card_cvv), secret.cardCvv, vm, hidden = true)
        }
        Mobile.TypeBinary -> {
            Text(stringResource(R.string.type_binary), style = MaterialTheme.typography.labelMedium)
            Text(Formatter.formatShortFileSize(context, secret.fileSize), style = MaterialTheme.typography.bodyLarge)
            OutlinedButton(
                onClick = {
                    pauseLock()
                    saveFile.launch(secret.ref.name)
                },
                enabled = !vm.busy,
                modifier = Modifier.fillMaxWidth(),
            ) { Text(stringResource(R.string.file_save)) }
        }
    }

    if (secret.metadata.isNotBlank()) {
        HorizontalDivider()
        Text(stringResource(R.string.secret_metadata), style = MaterialTheme.typography.labelMedium)
        Text(secret.metadata, style = MaterialTheme.typography.bodyMedium)
    }

    val dates = listOfNotNull(
        secret.createdAt.takeIf { it > 0 }?.let { stringResource(R.string.created_at, formatDate(it)) },
        secret.updatedAt.takeIf { it > 0 }?.let { stringResource(R.string.updated_at, formatDate(it)) },
    )
    if (dates.isNotEmpty()) {
        HorizontalDivider()
        dates.forEach {
            Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** Поле секрета: значение (скрытое, пока его не показали) и кнопки «Показать» и «Копировать». */
@Composable
private fun Field(
    label: String,
    value: String,
    vm: SecretViewModel,
    hidden: Boolean = false,
    multiline: Boolean = false,
    display: String = value,
    masked: String = MASK,
) {
    if (value.isEmpty()) return
    var revealed by remember { mutableStateOf(false) }
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(
            if (hidden && !revealed) masked else display,
            style = MaterialTheme.typography.bodyLarge,
            fontFamily = if (multiline) null else FontFamily.Monospace,
        )
        Row {
            if (hidden) {
                TextButton(onClick = { revealed = !revealed }) {
                    Text(stringResource(if (revealed) R.string.hide_password else R.string.show_password))
                }
            }
            TextButton(onClick = { vm.copy(label, value) }) { Text(stringResource(R.string.copy)) }
        }
    }
}

private fun formatDate(millis: Long): String =
    DateFormat.getDateTimeInstance(DateFormat.MEDIUM, DateFormat.SHORT).format(Date(millis))
