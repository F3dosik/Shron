package io.github.f3dosik.shron.autofill

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import android.view.autofill.AutofillId
import android.view.autofill.AutofillManager
import android.view.autofill.AutofillValue
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.annotation.RequiresApi
import androidx.core.content.IntentCompat
import androidx.fragment.app.FragmentActivity
import io.github.f3dosik.shron.MainActivity
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.autofill.ShronAutofillService.Companion.setValueCompat
import io.github.f3dosik.shron.ui.secrets.SecretDetails
import io.github.f3dosik.shron.ui.theme.ShronTheme

/**
 * Окно выбора логина, которое система открывает по пункту «Shron» в автозаполнении.
 * Возвращает Dataset с логином и паролем выбранного секрета; отмена — ничего не заполняет.
 */
@RequiresApi(Build.VERSION_CODES.O)
class AutofillPickerActivity : FragmentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        enableEdgeToEdge()
        setResult(Activity.RESULT_CANCELED)

        val usernameId = IntentCompat.getParcelableExtra(intent, EXTRA_USERNAME_ID, AutofillId::class.java)
        val passwordId = IntentCompat.getParcelableExtra(intent, EXTRA_PASSWORD_ID, AutofillId::class.java)
        val webDomain = intent.getStringExtra(EXTRA_WEB_DOMAIN)
        val targetPackage = intent.getStringExtra(EXTRA_PACKAGE)
        if (usernameId == null && passwordId == null) {
            finish()
            return
        }

        val app = application as ShronApplication
        setContent {
            ShronTheme {
                AutofillPicker(
                    app = app,
                    requester = webDomain ?: appLabel(targetPackage),
                    keyword = searchKeyword(webDomain, targetPackage),
                    onFill = { secret -> fill(secret, usernameId, passwordId) },
                    onOpenApp = {
                        startActivity(Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                        finish()
                    },
                    onCancel = ::finish,
                )
            }
        }
    }

    private fun fill(secret: SecretDetails, usernameId: AutofillId?, passwordId: AutofillId?) {
        val dataset = ShronAutofillService.datasetBuilder(this, secret.ref.name)
        usernameId?.let { dataset.setValueCompat(it, AutofillValue.forText(secret.login)) }
        passwordId?.let { dataset.setValueCompat(it, AutofillValue.forText(secret.password)) }
        setResult(Activity.RESULT_OK, Intent().putExtra(AutofillManager.EXTRA_AUTHENTICATION_RESULT, dataset.build()))
        finish()
    }

    /** Название приложения, которое просит заполнить форму; без доступа к нему — имя пакета. */
    private fun appLabel(pkg: String?): String {
        if (pkg == null) return ""
        return try {
            packageManager.getApplicationLabel(packageManager.getApplicationInfo(pkg, 0)).toString()
        } catch (_: PackageManager.NameNotFoundException) {
            pkg
        }
    }

    companion object {
        const val EXTRA_USERNAME_ID = "io.github.f3dosik.shron.autofill.USERNAME_ID"
        const val EXTRA_PASSWORD_ID = "io.github.f3dosik.shron.autofill.PASSWORD_ID"
        const val EXTRA_WEB_DOMAIN = "io.github.f3dosik.shron.autofill.WEB_DOMAIN"
        const val EXTRA_PACKAGE = "io.github.f3dosik.shron.autofill.PACKAGE"
    }
}
