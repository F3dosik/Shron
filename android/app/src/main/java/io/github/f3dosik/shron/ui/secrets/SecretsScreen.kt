package io.github.f3dosik.shron.ui.secrets

import androidx.annotation.StringRes
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.TextButton
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ui.common.OfflineNotice
import io.github.f3dosik.shron.mobile.Mobile
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SecretsScreen(
    vm: SecretsViewModel,
    onAdd: () -> Unit,
    onOpen: (SecretRef) -> Unit,
    onSettings: () -> Unit,
    onConflicts: () -> Unit,
) {
    // Список загружается при каждом показе экрана (после разблокировки, после добавления
    // секрета) и забывается при уходе.
    DisposableEffect(Unit) {
        vm.load()
        onDispose(vm::forget)
    }
    var menu by remember { mutableStateOf(false) }
    var confirmLogout by remember { mutableStateOf(false) }
    val coreState by vm.coreState.collectAsStateWithLifecycle()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.vault_title)) },
                actions = {
                    IconButton(onClick = vm::lock) {
                        Icon(Icons.Filled.Lock, contentDescription = stringResource(R.string.action_lock))
                    }
                    Box {
                        IconButton(onClick = { menu = true }) {
                            Icon(Icons.Filled.MoreVert, contentDescription = stringResource(R.string.action_more))
                        }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.settings_title)) },
                                onClick = { menu = false; onSettings() },
                            )
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.logout)) },
                                onClick = {
                                    menu = false
                                    if (coreState.pendingChanges > 0) confirmLogout = true else vm.logout()
                                },
                            )
                        }
                    }
                },
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = onAdd) {
                Icon(Icons.Filled.Add, contentDescription = stringResource(R.string.action_add_secret))
            }
        },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            if (coreState.offline) OfflineBanner(coreState.syncedAt, coreState.pendingChanges)
            if (coreState.conflicts > 0) ConflictsBanner(coreState.conflicts, onConflicts)
            PullToRefreshBox(
                isRefreshing = vm.busy,
                onRefresh = vm::refresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                val all = vm.items
                val error = vm.error
                when {
                    error != null && all.isNullOrEmpty() -> Placeholder(error.message, isError = true)
                    all == null -> Unit
                    all.isEmpty() -> Placeholder(stringResource(R.string.secrets_empty))
                    else -> LazyColumn(Modifier.fillMaxSize()) {
                        item { SearchBar(vm) }
                        if (error != null) {
                            item { Text(error.message, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) }
                        }
                        val items = filterSecrets(all, vm.query, vm.typeFilter)
                        if (items.isEmpty()) {
                            item {
                                Text(
                                    stringResource(R.string.search_nothing_found),
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.padding(32.dp),
                                )
                            }
                        }
                        // Пока групп нет, список плоский, без заголовков. При поиске группы
                        // раскрыты, чтобы найденное не пряталось в свёрнутых.
                        val headers = all.any { it.group.isNotEmpty() }
                        val searching = vm.query.isNotBlank()
                        groupSecrets(items).forEach { group ->
                            val expanded = !headers || searching || group.name !in vm.collapsed
                            if (headers) {
                                stickyHeader(key = "group/" + group.name) {
                                    GroupHeader(group, expanded, enabled = !searching, onClick = { vm.toggleGroup(group.name) })
                                }
                            }
                            if (!expanded) return@forEach
                            items(group.items, key = { it.type + "/" + it.name }) { item ->
                                SecretRow(item, onClick = { onOpen(SecretRef(item.name, item.type)) })
                                HorizontalDivider()
                            }
                        }
                        // Место под круглую кнопку «+», чтобы она не закрывала последнюю запись.
                        item { Spacer(Modifier.height(88.dp)) }
                    }
                }
            }
        }
    }

    if (confirmLogout) {
        AlertDialog(
            onDismissRequest = { confirmLogout = false },
            title = { Text(stringResource(R.string.logout_pending_title)) },
            text = { Text(stringResource(R.string.logout_pending_text, coreState.pendingChanges)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmLogout = false
                    vm.logout()
                }) { Text(stringResource(R.string.logout_anyway), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = {
                TextButton(onClick = { confirmLogout = false }) { Text(stringResource(R.string.cancel)) }
            },
        )
    }
}

/**
 * Плашка «нет связи»: список и автозаполнение работают по копии на устройстве от
 * [syncedAt], изменения копятся на телефоне ([pending]) и уйдут при подключении.
 * Потянуть список — проверить связь.
 */
@Composable
private fun OfflineBanner(syncedAt: Long, pending: Int) {
    var text = if (syncedAt > 0) {
        val date = DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT).format(Date(syncedAt))
        stringResource(R.string.offline_banner, date)
    } else {
        stringResource(R.string.offline_banner_no_date)
    }
    if (pending > 0) text += "\n" + stringResource(R.string.offline_pending, pending)
    OfflineNotice(text)
}

@Composable
private fun SearchBar(vm: SecretsViewModel) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        OutlinedTextField(
            value = vm.query,
            onValueChange = { vm.query = it },
            placeholder = { Text(stringResource(R.string.search)) },
            leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
            trailingIcon = if (vm.query.isNotEmpty()) {
                { IconButton(onClick = { vm.query = "" }) { Icon(Icons.Filled.Clear, stringResource(R.string.search_clear)) } }
            } else null,
            singleLine = true,
            keyboardOptions = KeyboardOptions(autoCorrectEnabled = false, imeAction = ImeAction.Search),
            modifier = Modifier.weight(1f),
        )
        TypeFilter(vm.typeFilter, onChange = { vm.typeFilter = it })
    }
}

/** Фильтр по типу: один чип с текущим типом, по нажатию — меню типов. */
@Composable
private fun TypeFilter(type: String?, onChange: (String?) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    val current = type?.let { typeShortLabel(it) }?.let { stringResource(it) } ?: stringResource(R.string.filter_all)
    Box {
        FilterChip(
            selected = type != null,
            onClick = { expanded = true },
            label = { Text(stringResource(R.string.filter_type, current), maxLines = 1) },
            trailingIcon = { Icon(Icons.Filled.ArrowDropDown, contentDescription = null) },
            // Высота и форма — как у поля поиска рядом.
            shape = OutlinedTextFieldDefaults.shape,
            modifier = Modifier.height(OutlinedTextFieldDefaults.MinHeight),
        )
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            (listOf<String?>(null) + SECRET_TYPES).forEach { option ->
                DropdownMenuItem(
                    text = {
                        Text(option?.let { stringResource(typeLabel(it)!!) } ?: stringResource(R.string.filter_all_types))
                    },
                    trailingIcon = if (option == type) {
                        { Icon(Icons.Filled.Check, contentDescription = null) }
                    } else null,
                    onClick = {
                        expanded = false
                        onChange(option)
                    },
                )
            }
        }
    }
}

private val SECRET_TYPES = listOf(Mobile.TypeCredentials, Mobile.TypeText, Mobile.TypeCard, Mobile.TypeBinary)

@StringRes
private fun typeShortLabel(type: String): Int? = when (type) {
    Mobile.TypeCredentials -> R.string.type_credentials_short
    Mobile.TypeText -> R.string.type_text_short
    Mobile.TypeCard -> R.string.type_card_short
    Mobile.TypeBinary -> R.string.type_binary_short
    else -> null
}

@Composable
private fun GroupHeader(group: SecretGroup, expanded: Boolean, enabled: Boolean, onClick: () -> Unit) {
    val name = group.name.ifEmpty { stringResource(R.string.group_none) }
    val action = stringResource(if (expanded) R.string.group_collapse else R.string.group_expand)
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainer,
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = enabled, onClickLabel = action, onClick = onClick),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
        ) {
            Icon(
                if (expanded) Icons.Filled.KeyboardArrowUp else Icons.Filled.KeyboardArrowDown,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.primary,
            )
            Text(
                name,
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Text(
                group.items.size.toString(),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun SecretRow(item: SecretItem, onClick: () -> Unit) {
    ListItem(
        modifier = Modifier.clickable(onClick = onClick),
        overlineContent = { Text(typeLabel(item.type)?.let { stringResource(it) } ?: item.type) },
        headlineContent = { Text(item.name) },
        supportingContent = item.metadata.takeIf { it.isNotBlank() }?.let { { Text(it, maxLines = 2) } },
        trailingContent = {
            Column(horizontalAlignment = Alignment.End) {
                if (item.updatedAt > 0) Text(DateFormat.getDateInstance(DateFormat.SHORT).format(Date(item.updatedAt)))
                when (item.status) {
                    Mobile.StatusPending -> Text(
                        stringResource(R.string.status_pending),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Mobile.StatusConflict -> Text(
                        stringResource(R.string.status_conflict),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        },
    )
}

/** Плашка конфликтов: по нажатию — экран, где для каждой записи выбирается версия. */
@Composable
private fun ConflictsBanner(count: Int, onClick: () -> Unit) {
    Surface(
        color = MaterialTheme.colorScheme.errorContainer,
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
        ) {
            Icon(Icons.Filled.Warning, contentDescription = null, tint = MaterialTheme.colorScheme.onErrorContainer)
            Text(
                stringResource(R.string.conflicts_banner, count),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onErrorContainer,
                modifier = Modifier.weight(1f),
            )
            Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null, tint = MaterialTheme.colorScheme.onErrorContainer)
        }
    }
}

/** Сообщение на месте списка; LazyColumn — чтобы работал жест обновления. */
@Composable
private fun Placeholder(text: String, isError: Boolean = false) {
    LazyColumn(Modifier.fillMaxSize(), horizontalAlignment = Alignment.CenterHorizontally) {
        item {
            Text(
                text,
                color = if (isError) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(32.dp),
            )
        }
    }
}

@StringRes
internal fun typeLabel(type: String): Int? = when (type) {
    Mobile.TypeCredentials -> R.string.type_credentials
    Mobile.TypeText -> R.string.type_text
    Mobile.TypeCard -> R.string.type_card
    Mobile.TypeBinary -> R.string.type_binary
    else -> null
}
