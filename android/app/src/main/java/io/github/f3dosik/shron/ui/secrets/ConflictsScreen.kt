package io.github.f3dosik.shron.ui.secrets

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
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
import androidx.compose.ui.unit.dp
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.mobile.Mobile
import java.text.DateFormat
import java.util.Date

/**
 * Конфликты изменений, сделанных без сети: для каждой записи — что случилось, когда
 * её меняли на телефоне и на сервере, какие поля различаются, и решения.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConflictsScreen(vm: ConflictsViewModel, onBack: () -> Unit) {
    BackHandler(onBack = onBack)
    LaunchedEffect(Unit) { vm.load() }
    val items = vm.items
    // Все конфликты разрешены — вернуться к списку.
    LaunchedEffect(items) { if (items != null && items.isEmpty()) onBack() }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.conflicts_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
            )
        },
    ) { padding ->
        LazyColumn(
            Modifier
                .fillMaxSize()
                .padding(padding),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            vm.error?.let { error ->
                item { Text(error.message, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) }
            }
            if (items != null && items.size > 1) {
                item {
                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(horizontal = 8.dp)) {
                        Checkbox(checked = vm.applyToAll, onCheckedChange = { vm.applyToAll = it })
                        Text(stringResource(R.string.conflict_apply_all, items.size))
                    }
                }
            }
            items(items.orEmpty(), key = { it.ref.type + "/" + it.ref.name }) { item ->
                ConflictCard(item, enabled = !vm.busy, onResolve = { vm.resolve(item, it) })
            }
        }
    }
}

private fun formatTime(millis: Long): String =
    DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT).format(Date(millis))

@Composable
private fun ConflictCard(item: ConflictItem, enabled: Boolean, onResolve: (String) -> Unit) {
    var reveal by remember { mutableStateOf(false) }
    Card(Modifier
        .fillMaxWidth()
        .padding(horizontal = 16.dp)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(item.ref.name, style = MaterialTheme.typography.titleMedium)
            Text(conflictDescription(item))
            if (item.differences.isNotEmpty()) {
                Text(
                    stringResource(R.string.conflict_differences, item.differences),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (item.mine != null || item.server != null) {
                TextButton(onClick = { reveal = !reveal }) {
                    Text(stringResource(if (reveal) R.string.conflict_hide else R.string.conflict_show))
                }
            }
            if (reveal) {
                item.mine?.let { VersionDetails(stringResource(R.string.conflict_version_mine), item.ref.type, it) }
                item.server?.let { VersionDetails(stringResource(R.string.conflict_version_server), item.ref.type, it) }
            }
            Resolutions(item, enabled, onResolve)
        }
    }
}

@Composable
private fun conflictDescription(item: ConflictItem): String {
    val edited = formatTime(item.editedAt)
    val server = if (item.serverUpdatedAt > 0) formatTime(item.serverUpdatedAt) else ""
    return when (item.kind) {
        Mobile.ConflictChanged -> stringResource(R.string.conflict_changed, server, edited)
        Mobile.ConflictDeletedOnServer -> stringResource(R.string.conflict_deleted_on_server, edited)
        Mobile.ConflictExists -> stringResource(R.string.conflict_exists, edited, server)
        Mobile.ConflictChangedOnServer -> stringResource(R.string.conflict_changed_on_server, edited, server)
        else -> stringResource(R.string.conflict_rejected, item.message)
    }
}

/** Кнопки решений; названия зависят от вида конфликта. */
@Composable
private fun Resolutions(item: ConflictItem, enabled: Boolean, onResolve: (String) -> Unit) {
    val (mine, server) = when (item.kind) {
        Mobile.ConflictDeletedOnServer -> R.string.resolve_restore_mine to R.string.resolve_delete
        Mobile.ConflictChangedOnServer -> R.string.resolve_delete_anyway to R.string.resolve_keep_server_version
        Mobile.ConflictRejected -> R.string.resolve_retry to R.string.resolve_discard
        else -> R.string.resolve_mine to R.string.resolve_server
    }
    Column(verticalArrangement = Arrangement.spacedBy(4.dp), modifier = Modifier.fillMaxWidth()) {
        Button(onClick = { onResolve(Mobile.ResolveMine) }, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(mine))
        }
        OutlinedButton(onClick = { onResolve(Mobile.ResolveServer) }, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(server))
        }
        if (item.canKeepBoth) {
            OutlinedButton(onClick = { onResolve(Mobile.ResolveBoth) }, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.resolve_both))
            }
        }
    }
}

@Composable
private fun VersionDetails(title: String, type: String, v: ConflictVersion) {
    val rows = buildList {
        when (type) {
            Mobile.TypeCredentials -> {
                add(R.string.secret_login to v.login)
                add(R.string.secret_password to v.password)
                add(R.string.secret_uris to v.uris)
            }
            Mobile.TypeText -> add(R.string.secret_text to v.text)
            Mobile.TypeCard -> {
                add(R.string.card_number to v.cardNumber)
                add(R.string.card_holder to v.cardHolder)
                add(R.string.card_expiry_short to v.cardExpiry)
                add(R.string.card_cvv to v.cardCvv)
            }
        }
        add(R.string.secret_metadata to v.metadata)
        add(R.string.secret_group to v.group)
    }.filter { it.second.isNotEmpty() }
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(title, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
        rows.forEach { (label, value) ->
            Text(
                stringResource(label) + ": " + value,
                style = MaterialTheme.typography.bodySmall,
            )
        }
    }
}
