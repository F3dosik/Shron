package io.github.f3dosik.shron.autofill

import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import androidx.activity.compose.setContent
import androidx.annotation.RequiresApi
import androidx.fragment.app.FragmentActivity
import io.github.f3dosik.shron.MainActivity
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.ui.theme.ShronTheme
import kotlinx.coroutines.launch

/**
 * Пользователь согласился сохранить логин в Shron, а хранилище заблокировано: окно
 * разблокировки поверх приложения, после неё логин сохраняется (LoginSaver). Сам логин
 * ждёт в памяти процесса (LoginSaver.pending), а не в extras: PendingIntent хранит их в
 * системе.
 */
@RequiresApi(Build.VERSION_CODES.O)
class AutofillSaveActivity : FragmentActivity() {

    private var saving = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)

        val save = LoginSaver.pending
        if (save == null) {
            finish()
            return
        }
        val app = application as ShronApplication
        val requester = save.webDomain?.takeIf { it.isNotBlank() } ?: appLabel(save.packageName)
        setContent {
            ShronTheme {
                UnlockSheet(
                    app = app,
                    subtitle = getString(R.string.autofill_save_for, requester),
                    onUnlocked = { save(app, save) },
                    onOpenApp = {
                        startActivity(Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                        finish()
                    },
                    onCancel = {
                        LoginSaver.pending = null
                        finish()
                    },
                )
            }
        }
    }

    /** Сохранение — вне окна: оно закрывается сразу, итог покажет уведомление. */
    private fun save(app: ShronApplication, save: PendingSave) {
        if (saving) return
        saving = true
        LoginSaver.pending = null
        val context = applicationContext
        LoginSaver.scope.launch { LoginSaver.toast(context, LoginSaver.save(context, app.core, save)) }
        finish()
    }
}
