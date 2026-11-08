package io.github.f3dosik.shron.ui.account

import android.content.ContentResolver
import android.net.Uri
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.ui.common.CoreViewModel
import io.github.f3dosik.shron.ui.common.PickedFile
import io.github.f3dosik.shron.ui.common.readPickedFile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Экспорт хранилища в файл .gkx и импорт из него — как на десктопе и в CLI. Файл
 * выбирается системным диалогом (Storage Access Framework): Загрузки, облако, флешка.
 * Ошибки показываются в диалогах экспорта и импорта, поэтому у них своя ViewModel.
 */
class TransferViewModel(app: ShronApplication) : CoreViewModel(app) {

    var exportPassword by mutableStateOf("")
    var exportConfirm by mutableStateOf("")

    /** Зашифрованный файл экспорта, ждущий выбора места сохранения. */
    private var exportData: ByteArray? = null

    /** Выбранный файл для импорта; не null — показывается диалог пароля. */
    var importFile by mutableStateOf<PickedFile?>(null)
        private set
    var importPassword by mutableStateOf("")
    var importOverwrite by mutableStateOf(false)

    /** Итог для Snackbar. */
    var notice by mutableStateOf<String?>(null)
        private set

    fun noticeShown() {
        notice = null
    }

    fun resetExport() {
        exportPassword = ""
        exportConfirm = ""
        exportData = null
        dismissError()
    }

    /**
     * Шифрует хранилище паролем экспорта и передаёт [onReady] имя файла по умолчанию —
     * экран открывает диалог сохранения, а потом вызывает [saveExport].
     */
    fun prepareExport(onReady: (String) -> Unit) = run {
        if (exportPassword != exportConfirm) throw validation(R.string.passwords_mismatch)
        exportData = core.call { it.exportVault(exportPassword) }
        val name = core.call { it.exportFileName() }
        exportPassword = ""
        exportConfirm = ""
        onReady(name)
    }

    fun saveExport(resolver: ContentResolver, uri: Uri?) = run {
        val data = exportData ?: return@run
        exportData = null
        if (uri == null) return@run
        withContext(Dispatchers.IO) {
            resolver.openOutputStream(uri, "wt")?.use { it.write(data) }
        } ?: throw validation(R.string.error_write_file)
        notice = string(R.string.export_saved)
    }

    fun pickImport(resolver: ContentResolver, uri: Uri) = run {
        val file = readPickedFile(resolver, uri, MAX_IMPORT_SIZE) ?: throw validation(R.string.error_read_file)
        if (file.data.size > MAX_IMPORT_SIZE) throw validation(R.string.import_too_large)
        importPassword = ""
        importOverwrite = false
        importFile = file
    }

    fun cancelImport() {
        importFile = null
        importPassword = ""
        dismissError()
    }

    fun confirmImport() = run {
        val file = importFile ?: return@run
        val r = core.call { it.importVault(file.data, importPassword, importOverwrite) }
        importFile = null
        importPassword = ""
        var text = string(
            R.string.import_result,
            r.created.toInt(), r.updated.toInt(), r.skipped.toInt(), r.failed.toInt(),
        )
        if (r.interrupted) text += " " + string(R.string.import_interrupted, r.interruptReason)
        notice = text
    }

    private companion object {
        /** Файл экспорта целиком читается в память; записи с файлами весят до ~1 МБ. */
        const val MAX_IMPORT_SIZE = 256 shl 20
    }
}
