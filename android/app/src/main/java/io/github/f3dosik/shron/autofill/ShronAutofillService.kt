package io.github.f3dosik.shron.autofill

import android.app.assist.AssistStructure
import android.content.Context
import android.os.Build
import android.os.CancellationSignal
import android.service.autofill.AutofillService
import android.service.autofill.Dataset
import android.service.autofill.FillCallback
import android.service.autofill.FillRequest
import android.service.autofill.FillResponse
import android.service.autofill.SaveCallback
import android.service.autofill.SaveRequest
import android.view.View
import android.view.autofill.AutofillId
import android.view.autofill.AutofillValue
import android.widget.RemoteViews
import androidx.annotation.DrawableRes
import androidx.annotation.RequiresApi
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ShronApplication
import io.github.f3dosik.shron.core.ShronError
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * Служба автозаполнения. Пока хранилище заблокировано, на форму входа она отвечает одной
 * подсказкой «Разблокировать Shron» без данных. Когда хранилище открыто — логинами,
 * к которым привязан этот сайт или приложение (строгое сравнение доменов, см.
 * domain.MatchURI), и «Другим логином…» для выбора из всех. Значения логина приложение
 * получает, только когда пользователь выбрал его в подсказках.
 */
@RequiresApi(Build.VERSION_CODES.O)
class ShronAutofillService : AutofillService() {

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    override fun onFillRequest(request: FillRequest, cancellationSignal: CancellationSignal, callback: FillCallback) {
        val structure = request.fillContexts.lastOrNull()?.structure
        val target = structure?.activityComponent?.packageName
        // Свои экраны не заполняем: там мастер-пароль и формы записей.
        if (structure == null || target == null || target == packageName) {
            callback.onSuccess(null)
            return
        }

        // Поле, на котором вызвано автозаполнение: форма строится от него (см. parseLoginForm).
        val focusedId = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            request.fillContexts.lastOrNull()?.focusedId
        } else {
            null
        }
        val scan = Scan(focusedId)
        for (i in 0 until structure.windowNodeCount) scan.visit(structure.getWindowNodeAt(i).rootViewNode)
        val form = parseLoginForm(scan.fields)
        if (form == null) {
            callback.onSuccess(null)
            return
        }
        val inline = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) request.inlineSuggestionsRequest else null
        val autofill = AutofillTarget(form.username, form.password, scan.webDomain, target, inline)

        val core = (application as ShronApplication).core
        if (!core.state.value.unlocked) {
            callback.onSuccess(AutofillResponses.locked(this, autofill, request.id))
            return
        }
        val job = scope.launch {
            val response = try {
                AutofillResponses.logins(this@ShronAutofillService, autofill, request.id, core.matchLogins(scan.webDomain, target))
            } catch (_: ShronError) {
                // Сессия истекла, сервер недоступен и т.п.: хранилище могло заблокироваться.
                AutofillResponses.locked(this@ShronAutofillService, autofill, request.id)
            }
            callback.onSuccess(response)
        }
        cancellationSignal.setOnCancelListener { job.cancel() }
    }

    override fun onSaveRequest(request: SaveRequest, callback: SaveCallback) {
        // Сохранение новых логинов пока не поддерживается: SaveInfo в ответах не задаётся,
        // поэтому система сюда не обращается.
        callback.onSuccess()
    }

    /** Поля формы и домен сайта (для браузеров) из структуры экрана. */
    private class Scan(private val focusedId: AutofillId?) {
        val fields = mutableListOf<FormField<AutofillId>>()
        var webDomain: String? = null

        fun visit(node: AssistStructure.ViewNode) {
            if (webDomain == null && !node.webDomain.isNullOrBlank()) webDomain = node.webDomain
            val id = node.autofillId
            if (id != null && node.autofillType == View.AUTOFILL_TYPE_TEXT && node.visibility == View.VISIBLE) {
                fields += FormField(
                    id = id,
                    autofillHints = node.autofillHints?.toList().orEmpty(),
                    inputType = node.inputType,
                    idEntry = node.idEntry,
                    hint = node.hint,
                    html = node.htmlInfo?.attributes?.associate { it.first.lowercase() to it.second }.orEmpty(),
                    focused = if (focusedId != null) id == focusedId else node.isFocused,
                )
            }
            for (i in 0 until node.childCount) visit(node.getChildAt(i))
        }
    }

    companion object {
        /** Подсказка в списке под полем: значок, [text] и необязательная вторая строка. */
        fun presentation(
            context: Context,
            text: String,
            subtitle: String? = null,
            @DrawableRes icon: Int = R.drawable.ic_autofill_key,
        ) = RemoteViews(context.packageName, R.layout.autofill_item).apply {
            setImageViewResource(R.id.autofill_icon, icon)
            // Цвет значка — кодом: атрибут tint у ImageView в RemoteViews ненадёжен.
            setInt(R.id.autofill_icon, "setColorFilter", context.getColor(R.color.autofill_icon))
            setTextViewText(R.id.autofill_text, text)
            if (!subtitle.isNullOrEmpty()) {
                setTextViewText(R.id.autofill_subtitle, subtitle)
                setViewVisibility(R.id.autofill_subtitle, View.VISIBLE)
            }
        }

        // Dataset.Builder(RemoteViews) и setValue(id, value) устарели в API 33 (Presentations и
        // Field), но работают на всех версиях; новый API — только с 33, а minSdk службы — 26.
        @Suppress("DEPRECATION")
        fun datasetBuilder(
            context: Context,
            text: String,
            subtitle: String? = null,
            @DrawableRes icon: Int = R.drawable.ic_autofill_key,
        ) = Dataset.Builder(presentation(context, text, subtitle, icon))

        @Suppress("DEPRECATION")
        fun Dataset.Builder.setValueCompat(id: AutofillId, value: AutofillValue?): Dataset.Builder =
            setValue(id, value)
    }
}
