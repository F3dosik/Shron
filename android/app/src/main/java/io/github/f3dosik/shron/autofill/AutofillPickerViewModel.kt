package io.github.f3dosik.shron.autofill

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.mobile.Mobile
import io.github.f3dosik.shron.ui.common.CoreViewModel
import io.github.f3dosik.shron.ui.secrets.SecretDetails
import io.github.f3dosik.shron.ui.secrets.SecretItem
import io.github.f3dosik.shron.ui.secrets.filterSecrets

/** Выбор логина для автозаполнения: только секреты «логин и пароль». */
class AutofillPickerViewModel(app: ShronApplication, keyword: String) : CoreViewModel(app) {

    private var credentials by mutableStateOf<List<SecretItem>?>(null)

    /** Поиск заполнен словом из домена или пакета; пользователь может его стереть. */
    var query by mutableStateOf(keyword)

    val items: List<SecretItem>?
        get() = credentials?.let { filterSecrets(it, query, Mobile.TypeCredentials) }

    fun load() = run {
        credentials = core.listSecrets()
            .filter { it.type == Mobile.TypeCredentials }
            .map(SecretItem::from)
            .sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.name })
    }

    fun select(item: SecretItem, onSelected: (SecretDetails) -> Unit) = run {
        val secret = core.call { it.getSecret(item.name, item.type) }
        onSelected(SecretDetails.from(secret))
    }
}
