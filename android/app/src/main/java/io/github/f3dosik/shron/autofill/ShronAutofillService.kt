package io.github.f3dosik.shron.autofill

import android.app.PendingIntent
import android.app.assist.AssistStructure
import android.content.Context
import android.content.Intent
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
import androidx.core.os.BundleCompat
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
 *
 * После входа с новым логином или паролем система предлагает сохранить его в Shron
 * (SaveInfo в ответах, onSaveRequest): новая запись привязывается к сайту или приложению,
 * у существующей обновляется пароль.
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
        // Логин с прошлого шага входа (экран с почтой перед экраном с паролем).
        val previousUsername = request.clientState
            ?.let { BundleCompat.getParcelable(it, AutofillResponses.STATE_USERNAME_ID, AutofillId::class.java) }
            ?.takeIf { form.username == null }
        val autofill = AutofillTarget(form.username, form.password, scan.webDomain, target, inline, previousUsername)

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

    /**
     * Пользователь нажал «Сохранить» в предложении системы. Поля формы — из clientState
     * ответа, значения — из структур экранов (при входе в два шага логин на первом).
     * Хранилище открыто — логин сохраняется в фоне, заблокировано — сначала окно
     * разблокировки [AutofillSaveActivity].
     */
    override fun onSaveRequest(request: SaveRequest, callback: SaveCallback) {
        val contexts = request.fillContexts
        val target = contexts.lastOrNull()?.structure?.activityComponent?.packageName
        val state = request.clientState
        val passwordId = state?.let { BundleCompat.getParcelable(it, AutofillResponses.STATE_PASSWORD_ID, AutofillId::class.java) }
        val usernameId = state?.let { BundleCompat.getParcelable(it, AutofillResponses.STATE_USERNAME_ID, AutofillId::class.java) }
        if (target == null || target == packageName || passwordId == null) {
            callback.onSuccess()
            return
        }

        // Значения полей по всем экранам; более поздний экран важнее.
        val values = HashMap<AutofillId, String>()
        var webDomain: String? = null
        for (fill in contexts) {
            val screen = TextValues()
            for (i in 0 until fill.structure.windowNodeCount) screen.visit(fill.structure.getWindowNodeAt(i).rootViewNode)
            values.putAll(screen.values)
            if (screen.webDomain != null) webDomain = screen.webDomain
        }
        val password = values[passwordId].orEmpty()
        if (password.isEmpty()) {
            callback.onSuccess()
            return
        }
        val save = PendingSave(webDomain, target, usernameId?.let { values[it] }.orEmpty().trim(), password)

        val app = application as ShronApplication
        if (!app.core.state.value.unlocked) {
            LoginSaver.pending = save
            val intent = Intent(this, AutofillSaveActivity::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                val sender = PendingIntent.getActivity(
                    this, 0, intent, PendingIntent.FLAG_CANCEL_CURRENT or PendingIntent.FLAG_IMMUTABLE,
                ).intentSender
                callback.onSuccess(sender)
            } else {
                startActivity(intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                callback.onSuccess()
            }
            return
        }
        callback.onSuccess()
        val context = applicationContext
        LoginSaver.scope.launch { LoginSaver.toast(context, LoginSaver.save(context, app.core, save)) }
    }

    /** Непустые текстовые значения полей и домен сайта из структуры экрана. */
    private class TextValues {
        val values = HashMap<AutofillId, String>()
        var webDomain: String? = null

        fun visit(node: AssistStructure.ViewNode) {
            if (webDomain == null && !node.webDomain.isNullOrBlank()) webDomain = node.webDomain
            val id = node.autofillId
            val value = node.autofillValue
            if (id != null && value != null && value.isText) {
                val text = value.textValue.toString()
                if (text.isNotEmpty()) values[id] = text
            }
            for (i in 0 until node.childCount) visit(node.getChildAt(i))
        }
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
