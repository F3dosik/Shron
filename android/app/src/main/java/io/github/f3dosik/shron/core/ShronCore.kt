package io.github.f3dosik.shron.core

import io.github.f3dosik.shron.mobile.App
import io.github.f3dosik.shron.mobile.GeneratorOptions
import io.github.f3dosik.shron.mobile.LockListener
import io.github.f3dosik.shron.mobile.Mobile
import io.github.f3dosik.shron.mobile.SecretSummary
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withContext
import java.io.File

/** Снимок состояния Go-ядра (mobile.State) в неизменяемом виде для Compose. */
data class CoreState(
    val configured: Boolean = false,
    val serverAddress: String = "",
    val hasCACert: Boolean = false,
    val autoLockMinutes: Int = 0,
    val login: String = "",
    val unlocked: Boolean = false,
    /** Причина последней блокировки (Mobile.Lock*), пока хранилище заблокировано. */
    val lockReason: String? = null,
    /** Сервер недоступен: записи — из копии на устройстве, изменения невозможны. */
    val offline: Boolean = false,
    /** Когда копия на устройстве последний раз получила записи с сервера, мс Unix; 0 — нет копии. */
    val syncedAt: Long = 0,
    /** Изменения, сделанные без сети и ещё не отправленные; из них — с конфликтом. */
    val pendingChanges: Int = 0,
    val conflicts: Int = 0,
    /** Записи только на этом устройстве (и у заблокированного хранилища): при выходе или смене сервера удаляются. */
    val localRecords: Int = 0,
    /** Записи только на этом устройстве, зашифрованные прежним паролем (сменён на другом устройстве). */
    val staleLocalRecords: Int = 0,
)

/** Логин для подсказки автозаполнения — копия mobile.LoginMatch. */
data class LoginMatch(val name: String, val login: String, val password: String)

/** Сгенерированный пароль и его стойкость (strength — словами, от Go-ядра). */
data class GeneratedPassword(val password: String, val entropyBits: Int, val strength: String)

/**
 * Настройки генератора, как на десктопе: пароль из символов ([length], наборы символов)
 * или парольная фраза из [words] слов словаря EFF.
 */
data class GeneratorSettings(
    val passphrase: Boolean = false,
    val length: Int = 20,
    val words: Int = 6,
    val lower: Boolean = true,
    val upper: Boolean = true,
    val digits: Boolean = true,
    val symbols: Boolean = true,
    val noAmbiguous: Boolean = false,
) {
    companion object {
        // Диапазоны как в десктопе; ядро допускает до 128 символов и 12 слов.
        val LENGTH_RANGE = 8..64
        val WORDS_RANGE = 4..10
    }
}

/**
 * Единственный экземпляр Go-клиента на всё приложение. Методы ядра блокирующие (сеть,
 * Argon2id с 64 MiB памяти), поэтому все вызовы идут через [call] на Dispatchers.IO;
 * после каждого вызова обновляется [state], по которому интерфейс выбирает экран.
 */
class ShronCore(dataDir: File, insecure: Boolean) {

    private val _state = MutableStateFlow(CoreState())
    val state: StateFlow<CoreState> = _state.asStateFlow()

    // OnLock приходит из потока Go; MutableStateFlow потокобезопасен.
    private val app = App(dataDir.absolutePath, insecure, object : LockListener {
        override fun onLock(reason: String) = refresh(lockReason = reason)
    })

    init {
        refresh()
    }

    /** Выполняет [block] над ядром в фоне; ошибки ядра приходят как [ShronError]. */
    suspend fun <T> call(block: (App) -> T): T = withContext(Dispatchers.IO) {
        try {
            block(app)
        } catch (e: Exception) {
            throw ShronError.from(e)
        } finally {
            refresh()
        }
    }

    suspend fun listSecrets(): List<SecretSummary> = call { app ->
        val list = app.listSecrets()
        (0L until list.len()).mapNotNull { list.get(it) }
    }

    /** Логины, привязанные к сайту [webDomain] или приложению [packageName]. */
    suspend fun matchLogins(webDomain: String?, packageName: String): List<LoginMatch> = call { app ->
        val list = app.matchLogins(webDomain.orEmpty(), packageName)
        (0L until list.len()).mapNotNull { list.get(it) }.map { LoginMatch(it.name, it.login, it.password) }
    }

    /** Пароль или парольная фраза по настройкам генератора; генерация — на устройстве. */
    suspend fun generate(settings: GeneratorSettings): GeneratedPassword = withContext(Dispatchers.Default) {
        try {
            val g = if (settings.passphrase) {
                Mobile.generatePassphrase(settings.words.toLong())
            } else {
                Mobile.generatePassword(GeneratorOptions().apply {
                    length = settings.length.toLong()
                    lower = settings.lower
                    upper = settings.upper
                    digits = settings.digits
                    symbols = settings.symbols
                    noAmbiguous = settings.noAmbiguous
                })
            }
            GeneratedPassword(g.password, g.entropyBits.toInt(), g.strength)
        } catch (e: Exception) {
            throw ShronError.from(e)
        }
    }

    /**
     * Блокирует хранилище, не дожидаясь корутины (Lock не ходит в сеть). [reason] —
     * причина для экрана разблокировки; у уже заблокированного хранилища прежняя причина
     * сохраняется.
     */
    fun lock(reason: String = Mobile.LockManual) {
        val wasUnlocked = app.state.unlocked
        app.lock()
        refresh(lockReason = if (wasUnlocked) reason else _state.value.lockReason)
    }

    private fun refresh(lockReason: String? = _state.value.lockReason) {
        val s = app.state
        _state.value = CoreState(
            configured = s.configured,
            serverAddress = s.serverAddress,
            hasCACert = s.hasCACert,
            autoLockMinutes = s.autoLockMinutes.toInt(),
            login = s.login,
            unlocked = s.unlocked,
            lockReason = if (s.unlocked) null else lockReason,
            offline = s.unlocked && s.offline,
            syncedAt = s.syncedAt,
            pendingChanges = if (s.unlocked) s.pendingChanges.toInt() else 0,
            conflicts = if (s.unlocked) s.conflicts.toInt() else 0,
            localRecords = s.localRecords.toInt(),
            staleLocalRecords = if (s.unlocked) s.staleLocalRecords.toInt() else 0,
        )
    }
}
