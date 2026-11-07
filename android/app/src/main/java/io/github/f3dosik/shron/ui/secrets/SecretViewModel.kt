package io.github.f3dosik.shron.ui.secrets

import android.content.ContentResolver
import android.net.Uri
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.mobile.Secret
import io.github.f3dosik.shron.ui.common.CoreViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Ссылка на секрет: на сервере он определяется названием и типом. */
data class SecretRef(val name: String, val type: String)

/** Расшифрованный секрет — копия mobile.Secret, чтобы не держать ссылки на объекты Go. */
data class SecretDetails(
    val ref: SecretRef,
    val metadata: String,
    /** Группа; пусто — без группы. */
    val group: String = "",
    val createdAt: Long,
    val updatedAt: Long,
    val login: String = "",
    val password: String = "",
    /** Сайты и приложения логина, по одному на строку. */
    val uris: String = "",
    val text: String = "",
    val cardNumber: String = "",
    val cardHolder: String = "",
    val cardExpiry: String = "",
    val cardCvv: String = "",
    val fileSize: Long = 0,
) {
    companion object {
        fun from(s: Secret) = SecretDetails(
            ref = SecretRef(s.name, s.type),
            metadata = s.metadata,
            group = s.group,
            createdAt = s.createdAt,
            updatedAt = s.updatedAt,
            login = s.login,
            password = s.password,
            uris = s.urIs, // getURIs() из gomobile
            text = s.text,
            cardNumber = s.cardNumber,
            cardHolder = s.cardHolder,
            cardExpiry = s.cardExpiry,
            cardCvv = s.cardCVV,
            fileSize = s.fileSize,
        )
    }
}

/** Карточка секрета: содержимое запрашивается при открытии и живёт, пока карточка открыта. */
class SecretViewModel(app: ShronApplication, val ref: SecretRef) : CoreViewModel(app) {

    var secret by mutableStateOf<SecretDetails?>(null)
        private set

    /** Короткое сообщение для Snackbar («скопировано», «файл сохранён»). */
    var notice by mutableStateOf<String?>(null)
        private set

    init {
        load()
    }

    fun load() = run {
        secret = SecretDetails.from(core.call { it.getSecret(ref.name, ref.type) })
    }

    fun copy(label: String, value: String) {
        app.clipboard.copy(value)
        notice = string(R.string.copied, label)
    }

    fun noticeShown() {
        notice = null
    }

    fun saveFile(resolver: ContentResolver, uri: Uri) = run {
        val data = core.call { it.getFileContent(ref.name) }
        withContext(Dispatchers.IO) {
            resolver.openOutputStream(uri, "wt")?.use { it.write(data) }
        } ?: throw validation(R.string.error_write_file)
        notice = string(R.string.file_saved)
    }

    fun delete(onDeleted: () -> Unit) = run {
        core.call { it.deleteSecret(ref.name, ref.type) }
        onDeleted()
    }
}
