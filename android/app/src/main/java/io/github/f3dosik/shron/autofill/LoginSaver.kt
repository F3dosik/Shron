package io.github.f3dosik.shron.autofill

import android.content.Context
import android.content.pm.PackageManager
import android.widget.Toast
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.core.ShronCore
import io.github.f3dosik.shron.core.ShronError
import io.github.f3dosik.shron.mobile.Mobile
import io.github.f3dosik.shron.mobile.SecretInput
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob

/**
 * Сохраняет в хранилище логин, введённый в форму другого приложения или сайта (см.
 * [planSave]): новая запись привязывается к сайту или приложению, чтобы в следующий раз
 * сразу предлагаться в подсказках. Хранилище должно быть разблокировано.
 */
object LoginSaver {

    /**
     * Сохранение не привязано к жизни службы: система может отвязать её, пока идёт
     * запрос к серверу.
     */
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)

    /** Логин, ожидающий разблокировки хранилища в [AutofillSaveActivity]; только в памяти. */
    @Volatile
    var pending: PendingSave? = null

    /** Сохраняет логин и возвращает текст для уведомления пользователя. */
    suspend fun save(context: Context, core: ShronCore, save: PendingSave): String {
        val site = save.webDomain?.takeIf { it.isNotBlank() }
        val uri = bindingUri(site, save.packageName)
        val title = if (site != null) uri.orEmpty() else context.appLabel(save.packageName)
        return try {
            val matches = core.matchLogins(site, save.packageName)
            val names = core.listSecrets().map { it.name }
            when (val action = planSave(save, matches, names, title, uri)) {
                SaveAction.AlreadySaved -> context.getString(R.string.autofill_save_already)
                is SaveAction.Update -> {
                    val secret = core.call { it.getSecret(action.name, Mobile.TypeCredentials) }
                    val input = SecretInput().apply {
                        name = secret.name
                        type = secret.type
                        metadata = secret.metadata
                        group = secret.group
                        login = secret.login
                        password = action.password
                        urIs = secret.urIs // setURIs() из gomobile
                    }
                    core.call { it.saveSecret(input, true) }
                    context.getString(R.string.autofill_save_updated, action.name)
                }
                is SaveAction.Create -> {
                    val input = SecretInput().apply {
                        name = action.name
                        type = Mobile.TypeCredentials
                        login = action.login
                        password = action.password
                        urIs = action.uri.orEmpty()
                    }
                    core.call { it.saveSecret(input, false) }
                    context.getString(R.string.autofill_save_created, action.name)
                }
            }
        } catch (e: ShronError) {
            context.getString(R.string.autofill_save_failed, e.message)
        }
    }

    fun toast(context: Context, text: String) {
        Toast.makeText(context.applicationContext, text, Toast.LENGTH_LONG).show()
    }
}

/** Название приложения по пакету; без доступа к нему — имя пакета. */
fun Context.appLabel(pkg: String?): String {
    if (pkg == null) return ""
    return try {
        packageManager.getApplicationLabel(packageManager.getApplicationInfo(pkg, 0)).toString()
    } catch (_: PackageManager.NameNotFoundException) {
        pkg
    }
}
