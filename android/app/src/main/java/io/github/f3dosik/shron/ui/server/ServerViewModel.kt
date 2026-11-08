package io.github.f3dosik.shron.ui.server

import android.content.ContentResolver
import android.net.Uri
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.ui.common.CoreViewModel
import io.github.f3dosik.shron.ui.common.readPickedFile

/** Откуда брать CA-сертификат сервера. */
sealed interface CaSource {
    /** Системные корневые сертификаты (сервер с публичным сертификатом). */
    data object System : CaSource

    /** Сохранённый ранее на устройстве. */
    data object Saved : CaSource

    /** Выбранный файл (ca.crt). */
    data class File(val name: String, val pem: String) : CaSource
}

class ServerViewModel(app: ShronApplication) : CoreViewModel(app) {

    var address by mutableStateOf("")
    var autoLockMinutes by mutableStateOf("")
    var backgroundLockSeconds by mutableIntStateOf(app.preferences.backgroundLockSeconds)
    var ca by mutableStateOf<CaSource>(CaSource.System)
        private set
    var checkResult by mutableStateOf<String?>(null)
        private set

    /** Без сервера: записи только на телефоне. */
    var standalone by mutableStateOf(false)

    /** Выбор режима доступен, пока нет аккаунта на сервере (из него сначала нужно выйти). */
    val canChooseMode: Boolean get() = core.state.value.let { it.standalone || it.login.isEmpty() }

    init {
        val s = core.state.value
        address = s.serverAddress
        autoLockMinutes = if (s.configured) s.autoLockMinutes.toString() else DEFAULT_AUTO_LOCK.toString()
        ca = if (s.hasCACert) CaSource.Saved else CaSource.System
        standalone = s.standalone
    }

    fun useSystemCA() {
        ca = CaSource.System
        checkResult = null
    }

    fun loadCA(resolver: ContentResolver, uri: Uri) = run {
        checkResult = null
        val file = readPickedFile(resolver, uri, MAX_CA_SIZE) ?: throw validation(R.string.error_read_file)
        if (file.data.size > MAX_CA_SIZE) throw validation(R.string.server_ca_too_large)
        ca = CaSource.File(file.name, file.data.decodeToString())
    }

    fun check() = run {
        checkResult = null
        checkResult = core.call { it.checkServer(address, caPem(), ca == CaSource.Saved) }
    }

    fun save(onSaved: () -> Unit) = run {
        val minutes = autoLockMinutes.trim().toLongOrNull() ?: throw validation(R.string.server_auto_lock_invalid)
        if (standalone) {
            core.call { it.useStandalone(minutes) }
        } else {
            core.call { it.saveSettings(address, caPem(), ca == CaSource.Saved, minutes) }
        }
        app.preferences.backgroundLockSeconds = backgroundLockSeconds
        onSaved()
    }

    private fun caPem() = (ca as? CaSource.File)?.pem.orEmpty()

    private companion object {
        const val DEFAULT_AUTO_LOCK = 5
        const val MAX_CA_SIZE = 64 * 1024
    }
}
