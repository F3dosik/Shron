package io.github.f3dosik.shron.autofill

import android.app.Activity
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import android.view.autofill.AutofillManager
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.annotation.RequiresApi
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.f3dosik.shron.ui.common.accountName
import io.github.f3dosik.shron.MainActivity
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.core.LoginMatch
import io.github.f3dosik.shron.core.ShronError
import io.github.f3dosik.shron.ui.auth.AuthViewModel
import io.github.f3dosik.shron.ui.auth.BiometricUnlockButton
import io.github.f3dosik.shron.ui.auth.KdfDowngradeDialog
import io.github.f3dosik.shron.ui.common.ErrorMessage
import io.github.f3dosik.shron.ui.common.PasswordInput
import io.github.f3dosik.shron.ui.common.PrimaryButton
import io.github.f3dosik.shron.ui.theme.ShronTheme

/**
 * Разблокировка по подсказке «Разблокировать Shron»: небольшое окно поверх приложения,
 * которое заполняется, — сразу запрос биометрии, если она включена, иначе мастер-пароль.
 * После разблокировки возвращает системе подсказки с подходящими логинами
 * (AutofillResponses.logins), и пользователь выбирает логин, не открывая Shron.
 */
@RequiresApi(Build.VERSION_CODES.O)
class AutofillUnlockActivity : FragmentActivity() {

    private var responded = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        setResult(Activity.RESULT_CANCELED)

        val target = AutofillTarget.from(intent)
        if (target == null) {
            finish()
            return
        }
        val app = application as ShronApplication
        setContent {
            ShronTheme {
                UnlockSheet(
                    app = app,
                    subtitle = getString(R.string.autofill_unlock_for, target.webDomain ?: target.packageName),
                    onUnlocked = { respond(app, target) },
                    onOpenApp = {
                        startActivity(Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                        finish()
                    },
                    onCancel = ::finish,
                )
            }
        }
    }

    /** Подбирает логины и отдаёт системе новый ответ с ними вместо «Разблокировать Shron». */
    private suspend fun respond(app: ShronApplication, target: AutofillTarget) {
        if (responded) return
        responded = true
        val matches: List<LoginMatch> = try {
            app.core.matchLogins(target.webDomain, target.packageName)
        } catch (_: ShronError) {
            // Логины не получить (например, нет сети): останется «Выбрать логин» с поиском.
            emptyList()
        }
        val response = AutofillResponses.logins(this, target, nextRequestCode(), matches)
        setResult(Activity.RESULT_OK, Intent().putExtra(AutofillManager.EXTRA_AUTHENTICATION_RESULT, response))
        finish()
    }

    private companion object {
        /** Свой requestCode для PendingIntent каждого ответа — чтобы не перезаписать extras. */
        fun nextRequestCode() = (System.nanoTime() and 0x7fffffff).toInt()
    }
}

/**
 * Окно разблокировки поверх другого приложения. [subtitle] — для чего нужна разблокировка;
 * [onUnlocked] вызывается, когда хранилище открыто.
 */
@Composable
internal fun UnlockSheet(
    app: ShronApplication,
    subtitle: String,
    onUnlocked: suspend () -> Unit,
    onOpenApp: () -> Unit,
    onCancel: () -> Unit,
) {
    BackHandler(onBack = onCancel)
    val state by app.core.state.collectAsStateWithLifecycle()
    val auth: AuthViewModel = viewModel { AuthViewModel(app) }
    LaunchedEffect(state.unlocked) {
        if (state.unlocked) onUnlocked()
    }

    Box(
        contentAlignment = Alignment.Center,
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.4f))
            .imePadding()
            .padding(16.dp),
    ) {
        Surface(
            shape = RoundedCornerShape(24.dp),
            tonalElevation = 6.dp,
            modifier = Modifier
                .widthIn(max = 420.dp)
                .fillMaxWidth(),
        ) {
            Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(R.string.autofill_unlock_title), style = MaterialTheme.typography.headlineSmall)
                Text(
                    subtitle,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                when {
                    state.unlocked -> LinearProgressIndicator(Modifier.fillMaxWidth())
                    // Вход и временный пароль — в приложении: там полные экраны с подтверждением.
                    !state.signedIn || auth.temporaryPassword != null -> {
                        Text(stringResource(R.string.autofill_sign_in_first), style = MaterialTheme.typography.bodyMedium)
                        PrimaryButton(stringResource(R.string.autofill_open_app), busy = false, onClick = onOpenApp)
                    }
                    else -> UnlockForm(auth, accountName(state))
                }
                TextButton(onClick = onCancel, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.cancel)) }
            }
        }
    }
    KdfDowngradeDialog(auth)
}

@Composable
private fun UnlockForm(auth: AuthViewModel, login: String) {
    Text(
        stringResource(R.string.account_label, login),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    PasswordInput(
        value = auth.password,
        onValueChange = auth::updatePassword,
        label = stringResource(R.string.master_password),
        enabled = !auth.busy,
        onDone = auth::unlock,
    )
    ErrorMessage(auth.error)
    PrimaryButton(stringResource(R.string.unlock), busy = auth.busy, onClick = auth::unlock)
    // Пользователь сам нажал «Разблокировать Shron» — запрос биометрии сразу.
    BiometricUnlockButton(auth, autoPrompt = true)
}
