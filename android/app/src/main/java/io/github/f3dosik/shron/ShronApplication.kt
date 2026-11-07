package io.github.f3dosik.shron

import android.app.Application
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import io.github.f3dosik.shron.core.AppPreferences
import io.github.f3dosik.shron.core.BackgroundLock
import io.github.f3dosik.shron.core.BiometricUnlock
import io.github.f3dosik.shron.core.SecureClipboard
import io.github.f3dosik.shron.core.ShronCore

class ShronApplication : Application() {

    private val coreHolder = lazy { ShronCore(filesDir, BuildConfig.SHRON_INSECURE) }

    /** Go-ядро создаётся при первом обращении: загрузка библиотеки и настроек — не в onCreate. */
    val core: ShronCore by coreHolder

    val clipboard: SecureClipboard by lazy { SecureClipboard(this) }

    val preferences: AppPreferences by lazy { AppPreferences(this) }

    val biometric: BiometricUnlock by lazy { BiometricUnlock(this) }

    val backgroundLock = BackgroundLock(
        // elapsedRealtime идёт и во сне устройства, в отличие от uptimeMillis.
        clock = SystemClock::elapsedRealtime,
        scheduler = object : BackgroundLock.Scheduler {
            private val handler = Handler(Looper.getMainLooper())
            override fun schedule(delayMs: Long, action: Runnable) {
                handler.postDelayed(action, delayMs)
            }

            override fun cancel(action: Runnable) = handler.removeCallbacks(action)
        },
        delayMs = { preferences.backgroundLockSeconds * 1000L },
        // Ядро, которое ещё не создано, и так заблокировано.
        lock = { if (coreHolder.isInitialized()) core.lock(BackgroundLock.LOCK_REASON) },
    )

    override fun onCreate() {
        super.onCreate()
        // ProcessLifecycleOwner сообщает об уходе в фон всего приложения, а не одной Activity:
        // поворот экрана и переходы между Activity его не вызывают.
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStop(owner: LifecycleOwner) = backgroundLock.onBackground()
            override fun onStart(owner: LifecycleOwner) = backgroundLock.onForeground()
        })
    }
}
