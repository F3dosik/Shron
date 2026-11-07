package io.github.f3dosik.shron.ui.secrets

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.mobile.SecretSummary
import io.github.f3dosik.shron.ui.common.CoreViewModel

/** Строка списка секретов: копия SecretSummary, чтобы не держать ссылки на объекты Go. */
data class SecretItem(
    val name: String,
    val type: String,
    val metadata: String,
    val updatedAt: Long,
    /** Группа; пусто — без группы. */
    val group: String = "",
) {
    companion object {
        fun from(s: SecretSummary) = SecretItem(s.name, s.type, s.metadata, s.updatedAt, s.group)
    }
}

/** Секреты одной группы; [name] пустое — секреты без группы. */
data class SecretGroup(val name: String, val items: List<SecretItem>)

/**
 * Поиск как на десктопе: [query] — подстрока названия, комментария или группы без учёта
 * регистра, [type] — тип секрета (null — все).
 */
fun filterSecrets(items: List<SecretItem>, query: String, type: String?): List<SecretItem> {
    val q = query.trim()
    return items.filter { item ->
        (type == null || item.type == type) &&
            (q.isEmpty() || listOf(item.name, item.metadata, item.group).any { it.contains(q, ignoreCase = true) })
    }
}

/**
 * Раскладывает секреты по группам: группы по алфавиту, секреты без группы — последней.
 * Порядок секретов внутри группы сохраняется.
 */
fun groupSecrets(items: List<SecretItem>): List<SecretGroup> =
    items.groupBy { it.group }
        .map { (name, list) -> SecretGroup(name, list) }
        .sortedWith(compareBy<SecretGroup> { it.name.isEmpty() }.thenBy(String.CASE_INSENSITIVE_ORDER) { it.name })

/** Названия существующих групп по алфавиту — подсказки в форме секрета. */
fun groupNames(items: List<SecretItem>): List<String> =
    items.map { it.group }.filter { it.isNotEmpty() }.distinct().sortedWith(String.CASE_INSENSITIVE_ORDER)

class SecretsViewModel(app: ShronApplication) : CoreViewModel(app) {

    var items by mutableStateOf<List<SecretItem>?>(null)
        private set
    var query by mutableStateOf("")
    var typeFilter by mutableStateOf<String?>(null)

    /**
     * Свёрнутые группы. Переживают переход в секрет и обратно, но только в памяти:
     * названия групп не записываются на диск открытым текстом.
     */
    var collapsed by mutableStateOf(emptySet<String>())
        private set

    fun toggleGroup(name: String) {
        collapsed = if (name in collapsed) collapsed - name else collapsed + name
    }

    fun load() = run {
        items = core.listSecrets()
            .map(SecretItem::from)
            .sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.name })
    }

    /** Забывает список, когда хранилище блокируется. */
    fun forget() {
        items = null
        query = ""
        typeFilter = null
        dismissError()
    }

    fun lock() = core.lock()

    fun logout() = run {
        core.call { it.logout(false) }
        app.biometric.clear()
        collapsed = emptySet()
    }
}
