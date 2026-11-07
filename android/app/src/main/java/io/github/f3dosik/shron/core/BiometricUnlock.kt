package io.github.f3dosik.shron.core

import android.content.Context
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import android.util.Base64
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.core.content.edit
import java.security.GeneralSecurityException
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Мастер-ключ для разблокировки по биометрии. Он хранится зашифрованным AES-GCM ключом
 * Android Keystore, которым можно воспользоваться только после подтверждения сильной
 * биометрией (класс 3): это проверяет защищённое оборудование телефона, а не код
 * приложения, и сам ключ Keystore из него не извлекается. Новый отпечаток на телефоне
 * делает ключ Keystore недействительным — тогда снова нужен мастер-пароль.
 *
 * Ключ сохраняется для одной учётки на одном сервере: для другой учётки или адреса
 * разблокировка по биометрии не предлагается.
 */
class BiometricUnlock(private val context: Context) {

    // Здесь только шифротекст и IV; без ключа Keystore они бесполезны.
    private val prefs = context.getSharedPreferences("shron_biometric", Context.MODE_PRIVATE)

    /** На телефоне есть сильная биометрия и добавлен хотя бы один отпечаток или лицо. */
    fun isAvailable(): Boolean =
        BiometricManager.from(context).canAuthenticate(BIOMETRIC_STRONG) == BiometricManager.BIOMETRIC_SUCCESS

    /** Сохранён ключ учётки [login] на сервере [server]. */
    fun isEnabled(server: String, login: String): Boolean =
        login.isNotEmpty() && prefs.getString(KEY_SERVER, null) == server &&
            prefs.getString(KEY_LOGIN, null) == login && prefs.contains(KEY_DATA)

    /**
     * Шифр для сохранения ключа на новом ключе Keystore (прежний удаляется). Годен только
     * после подтверждения биометрией: его нужно передать в BiometricPrompt.
     */
    fun encryptCipher(): Cipher {
        clear()
        val spec = KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .setUserAuthenticationRequired(true)
            .setInvalidatedByBiometricEnrollment(true)
            .apply {
                // Подтверждение на каждую операцию и только биометрией, без PIN-кода.
                // До Android 11 так и есть по умолчанию.
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                    setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG)
                }
            }
            .build()
        val key = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE).run {
            init(spec)
            generateKey()
        }
        return Cipher.getInstance(TRANSFORMATION).apply { init(Cipher.ENCRYPT_MODE, key) }
    }

    /**
     * Шифр для расшифровки сохранённого ключа. null — ключ Keystore стал недействительным
     * (добавлен отпечаток, сброшена блокировка экрана) или его нет: сохранённое удаляется,
     * нужен пароль.
     */
    fun decryptCipher(): Cipher? {
        val iv = prefs.getString(KEY_IV, null)?.let(::decode) ?: return null
        return try {
            val key = keyStore().getKey(ALIAS, null) as? SecretKey ?: throw KeyPermanentlyInvalidatedException()
            Cipher.getInstance(TRANSFORMATION).apply { init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(TAG_BITS, iv)) }
        } catch (_: KeyPermanentlyInvalidatedException) {
            clear()
            null
        }
    }

    /** Шифрует и сохраняет [masterKey]; [cipher] — из [encryptCipher], подтверждённый биометрией. */
    fun save(cipher: Cipher, masterKey: ByteArray, server: String, login: String) {
        val data = cipher.doFinal(masterKey)
        prefs.edit {
            putString(KEY_SERVER, server)
            putString(KEY_LOGIN, login)
            putString(KEY_IV, encode(cipher.iv))
            putString(KEY_DATA, encode(data))
        }
    }

    /** Расшифровывает сохранённый ключ; [cipher] — из [decryptCipher], подтверждённый биометрией. */
    fun decrypt(cipher: Cipher): ByteArray {
        val data = prefs.getString(KEY_DATA, null)?.let(::decode) ?: throw GeneralSecurityException("no saved key")
        return cipher.doFinal(data)
    }

    /** Забывает ключ: при выходе из учётки, смене пароля или отключении в настройках. */
    fun clear() {
        prefs.edit { clear() }
        try {
            keyStore().deleteEntry(ALIAS)
        } catch (_: GeneralSecurityException) {
            // Ключа Keystore уже нет — забывать нечего.
        }
    }

    private fun keyStore() = KeyStore.getInstance(KEYSTORE).apply { load(null) }

    private companion object {
        const val KEYSTORE = "AndroidKeyStore"
        const val ALIAS = "shron_biometric_unlock"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val TAG_BITS = 128

        const val KEY_SERVER = "server"
        const val KEY_LOGIN = "login"
        const val KEY_IV = "iv"
        const val KEY_DATA = "data"

        fun encode(bytes: ByteArray): String = Base64.encodeToString(bytes, Base64.NO_WRAP)
        fun decode(text: String): ByteArray = Base64.decode(text, Base64.NO_WRAP)
    }
}
