package io.github.f3dosik.shron.autofill

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.core.ShronError
import io.github.f3dosik.shron.mobile.Mobile
import io.github.f3dosik.shron.mobile.SecretInput
import io.github.f3dosik.shron.ui.common.CoreViewModel
import io.github.f3dosik.shron.ui.secrets.SecretDetails
import io.github.f3dosik.shron.ui.secrets.SecretItem
import io.github.f3dosik.shron.ui.secrets.filterSecrets

/**
 * Выбор логина для автозаполнения: только записи-логины. [bindUri] — сайт или приложение
 * формы: выбранный логин можно к нему привязать, чтобы в следующий раз он сразу
 * предлагался в подсказках.
 */
class AutofillPickerViewModel(app: ShronApplication, keyword: String, val bindUri: String?) : CoreViewModel(app) {

    /** Привязать выбранный логин к [bindUri]. По умолчанию выключено: форма может быть поддельной. */
    var bind by mutableStateOf(false)

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
        val secret = SecretDetails.from(core.call { it.getSecret(item.name, item.type) })
        val uri = bindUri
        if (bind && uri != null) {
            val uris = addUri(secret.uris, uri)
            if (uris != secret.uris) {
                val input = SecretInput().apply {
                    name = secret.ref.name
                    type = secret.ref.type
                    metadata = secret.metadata
                    group = secret.group
                    login = secret.login
                    password = secret.password
                    urIs = uris // setURIs() из gomobile
                }
                // Не удалось привязать — всё равно заполняем: пользователь выбрал логин.
                try {
                    core.call { it.saveSecret(input, true) }
                } catch (_: ShronError) {
                }
            }
        }
        onSelected(secret)
    }
}
