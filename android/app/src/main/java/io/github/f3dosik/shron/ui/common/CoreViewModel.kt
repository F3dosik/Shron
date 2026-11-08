package io.github.f3dosik.shron.ui.common

import androidx.annotation.StringRes
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.core.CoreState
import io.github.f3dosik.shron.core.ShronCore
import io.github.f3dosik.shron.core.ShronError
import io.github.f3dosik.shron.mobile.Mobile
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

/**
 * ViewModel экрана, который вызывает Go-ядро: одна операция за раз, [busy] на время
 * вызова, ошибка ядра — в [error]. Поля форм хранятся во ViewModel, а не в
 * rememberSaveable, чтобы пароли не попадали в сохраняемое состояние Activity.
 */
abstract class CoreViewModel(protected val app: ShronApplication) : ViewModel() {

    protected val core: ShronCore get() = app.core

    /** Состояние ядра — например, чтобы показать, что сервер недоступен. */
    val coreState: StateFlow<CoreState> get() = core.state

    var busy by mutableStateOf(false)
        private set
    var error by mutableStateOf<ShronError?>(null)
        protected set

    protected fun run(block: suspend () -> Unit) {
        if (busy) return
        busy = true
        error = null
        viewModelScope.launch {
            try {
                block()
            } catch (e: ShronError) {
                onError(e)
            } finally {
                busy = false
            }
        }
    }

    /** Обработка ошибки; по умолчанию — показать её на экране. */
    protected open fun onError(e: ShronError) {
        error = e
    }

    fun dismissError() {
        error = null
    }

    protected fun string(@StringRes id: Int, vararg args: Any): String = app.getString(id, *args)

    /** Ошибка проверки формы с текстом из ресурсов. */
    protected fun validation(@StringRes id: Int) = ShronError(Mobile.CodeValidation, string(id))
}
