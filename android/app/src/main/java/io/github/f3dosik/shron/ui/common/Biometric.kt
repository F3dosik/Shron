package io.github.f3dosik.shron.ui.common

import android.content.Context
import android.content.ContextWrapper
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.biometric.BiometricPrompt
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import io.github.f3dosik.shron.R
import javax.crypto.Cipher

/**
 * Системный запрос биометрии для шифра Keystore: после подтверждения шифр становится
 * годным для одной операции. Отмена пользователем ничего не сообщает; другие ошибки
 * (слишком много попыток, датчик недоступен) приходят в onError текстом системы.
 */
class BiometricPrompter(private val activity: FragmentActivity) {

    fun authenticate(
        title: String,
        subtitle: String?,
        cipher: Cipher,
        onSuccess: (Cipher) -> Unit,
        onError: (String) -> Unit,
    ) {
        val callback = object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                result.cryptoObject?.cipher?.let(onSuccess) ?: onError(activity.getString(R.string.biometric_failed))
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                if (errorCode !in SILENT_ERRORS) onError(errString.toString())
            }
        }
        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle(title)
            .setSubtitle(subtitle)
            .setNegativeButtonText(activity.getString(R.string.biometric_use_password))
            .setAllowedAuthenticators(BIOMETRIC_STRONG)
            .setConfirmationRequired(false)
            .build()
        BiometricPrompt(activity, ContextCompat.getMainExecutor(activity), callback)
            .authenticate(info, BiometricPrompt.CryptoObject(cipher))
    }

    private companion object {
        val SILENT_ERRORS = setOf(
            BiometricPrompt.ERROR_USER_CANCELED,
            BiometricPrompt.ERROR_NEGATIVE_BUTTON,
            BiometricPrompt.ERROR_CANCELED,
        )
    }
}

@Composable
fun rememberBiometricPrompter(): BiometricPrompter {
    val activity = LocalContext.current.findFragmentActivity()
    return remember(activity) { BiometricPrompter(activity) }
}

private tailrec fun Context.findFragmentActivity(): FragmentActivity = when (this) {
    is FragmentActivity -> this
    is ContextWrapper -> baseContext.findFragmentActivity()
    else -> error("BiometricPrompt needs a FragmentActivity")
}
