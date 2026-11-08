package io.github.f3dosik.shron.ui.secrets

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.mobile.Conflict
import io.github.f3dosik.shron.mobile.Mobile
import io.github.f3dosik.shron.ui.common.CoreViewModel

/** Версия записи в конфликте — только поля, которые показываются пользователю. */
data class ConflictVersion(
    val login: String = "",
    val password: String = "",
    val uris: String = "",
    val text: String = "",
    val cardNumber: String = "",
    val cardHolder: String = "",
    val cardExpiry: String = "",
    val cardCvv: String = "",
    val metadata: String = "",
    val group: String = "",
)

/** Конфликт: копия mobile.Conflict, чтобы не держать ссылки на объекты Go. */
data class ConflictItem(
    val ref: SecretRef,
    val kind: String,
    val editedAt: Long,
    val serverUpdatedAt: Long,
    val message: String,
    val differences: String,
    val canKeepBoth: Boolean,
    val mine: ConflictVersion?,
    val server: ConflictVersion?,
) {
    companion object {
        fun from(c: Conflict) = ConflictItem(
            ref = SecretRef(c.name, c.type),
            kind = c.kind,
            editedAt = c.editedAt,
            serverUpdatedAt = c.serverUpdatedAt,
            message = c.message,
            differences = c.differences,
            canKeepBoth = c.canKeepBoth,
            mine = c.mine?.let(::version),
            server = c.server?.let(::version),
        )

        private fun version(s: io.github.f3dosik.shron.mobile.Secret) = ConflictVersion(
            login = s.login, password = s.password, uris = s.urIs, text = s.text,
            cardNumber = s.cardNumber, cardHolder = s.cardHolder, cardExpiry = s.cardExpiry, cardCvv = s.cardCVV,
            metadata = s.metadata, group = s.group,
        )
    }
}

class ConflictsViewModel(app: ShronApplication) : CoreViewModel(app) {

    var items by mutableStateOf<List<ConflictItem>?>(null)
        private set

    /** Применить выбранное решение ко всем конфликтам, где оно допустимо. */
    var applyToAll by mutableStateOf(false)

    fun load() = run { loadItems() }

    private suspend fun loadItems() {
        items = core.call { app ->
            val list = app.conflicts()
            (0L until list.len()).mapNotNull { list.get(it) }.map(ConflictItem::from)
        }
    }

    fun resolve(item: ConflictItem, choice: String) = run {
        val targets = if (applyToAll) {
            items.orEmpty().filter { choice != Mobile.ResolveBoth || it.canKeepBoth }
        } else {
            listOf(item)
        }
        try {
            for (t in targets) {
                core.call { it.resolveConflict(t.ref.name, t.ref.type, choice) }
            }
        } finally {
            loadItems()
        }
    }
}
