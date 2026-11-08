package io.github.f3dosik.shron.autofill

import android.annotation.SuppressLint
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.graphics.drawable.Icon
import android.os.Build
import android.os.Bundle
import android.service.autofill.Dataset
import android.service.autofill.FillResponse
import android.service.autofill.InlinePresentation
import android.service.autofill.SaveInfo
import android.view.autofill.AutofillId
import android.view.autofill.AutofillValue
import android.view.inputmethod.InlineSuggestionsRequest
import android.widget.inline.InlinePresentationSpec
import androidx.annotation.DrawableRes
import androidx.annotation.RequiresApi
import androidx.autofill.inline.UiVersions
import androidx.autofill.inline.v1.InlineSuggestionUi
import androidx.core.content.IntentCompat
import io.github.f3dosik.shron.MainActivity
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.autofill.ShronAutofillService.Companion.datasetBuilder
import io.github.f3dosik.shron.autofill.ShronAutofillService.Companion.setValueCompat
import io.github.f3dosik.shron.core.LoginMatch

/**
 * Заполняемая форма: поля логина и пароля, сайт (в браузере) или приложение, и запрос
 * подсказок над клавиатурой (Android 11+, если клавиатура их поддерживает). Передаётся
 * окнам Shron через extras неизменяемого PendingIntent.
 *
 * [previousUsernameId] — поле логина с прошлого шага входа, если вход в два шага (почта,
 * затем пароль на следующем экране): по нему логин найдётся при сохранении.
 */
@RequiresApi(Build.VERSION_CODES.O)
data class AutofillTarget(
    val usernameId: AutofillId?,
    val passwordId: AutofillId?,
    val webDomain: String?,
    val packageName: String,
    val inlineRequest: InlineSuggestionsRequest?,
    val previousUsernameId: AutofillId? = null,
) {
    val ids: List<AutofillId> get() = listOfNotNull(usernameId, passwordId)

    /** Поле логина для сохранения: на этом экране или на прошлом шаге входа. */
    val saveUsernameId: AutofillId? get() = usernameId ?: previousUsernameId

    fun putInto(intent: Intent): Intent = intent
        .putExtra(EXTRA_USERNAME_ID, usernameId)
        .putExtra(EXTRA_PASSWORD_ID, passwordId)
        .putExtra(EXTRA_WEB_DOMAIN, webDomain)
        .putExtra(EXTRA_PACKAGE, packageName)
        .putExtra(EXTRA_PREVIOUS_USERNAME_ID, previousUsernameId)
        .apply {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R && inlineRequest != null) {
                putExtra(EXTRA_INLINE_REQUEST, inlineRequest)
            }
        }

    companion object {
        private const val EXTRA_USERNAME_ID = "io.github.f3dosik.shron.autofill.USERNAME_ID"
        private const val EXTRA_PASSWORD_ID = "io.github.f3dosik.shron.autofill.PASSWORD_ID"
        private const val EXTRA_WEB_DOMAIN = "io.github.f3dosik.shron.autofill.WEB_DOMAIN"
        private const val EXTRA_PACKAGE = "io.github.f3dosik.shron.autofill.PACKAGE"
        private const val EXTRA_INLINE_REQUEST = "io.github.f3dosik.shron.autofill.INLINE_REQUEST"
        private const val EXTRA_PREVIOUS_USERNAME_ID = "io.github.f3dosik.shron.autofill.PREVIOUS_USERNAME_ID"

        /** null — в intent нет формы (окно открыто не системой автозаполнения). */
        fun from(intent: Intent): AutofillTarget? {
            val username = IntentCompat.getParcelableExtra(intent, EXTRA_USERNAME_ID, AutofillId::class.java)
            val password = IntentCompat.getParcelableExtra(intent, EXTRA_PASSWORD_ID, AutofillId::class.java)
            val pkg = intent.getStringExtra(EXTRA_PACKAGE)
            if ((username == null && password == null) || pkg == null) return null
            val inline = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                IntentCompat.getParcelableExtra(intent, EXTRA_INLINE_REQUEST, InlineSuggestionsRequest::class.java)
            } else {
                null
            }
            val previous = IntentCompat.getParcelableExtra(intent, EXTRA_PREVIOUS_USERNAME_ID, AutofillId::class.java)
            return AutofillTarget(username, password, intent.getStringExtra(EXTRA_WEB_DOMAIN), pkg, inline, previous)
        }
    }
}

/**
 * Ответы службы автозаполнения. Подсказки показываются списком под полем и, если
 * клавиатура умеет, — чипами над ней (inline, Android 11+).
 */
@RequiresApi(Build.VERSION_CODES.O)
object AutofillResponses {

    /**
     * Хранилище заблокировано: одна подсказка «Разблокировать Shron». Её выбор открывает
     * [AutofillUnlockActivity], которая после разблокировки возвращает ответ [logins].
     */
    fun locked(context: Context, target: AutofillTarget, requestId: Int): FillResponse {
        val unlock = Intent(context, AutofillUnlockActivity::class.java)
        val sender = pendingActivity(context, requestId, target.putInto(unlock)).intentSender
        val text = context.getString(R.string.autofill_unlock_chip)
        val presentation = ShronAutofillService.presentation(
            context, text, context.getString(R.string.autofill_unlock_subtitle), R.drawable.ic_autofill_lock,
        )
        val builder = FillResponse.Builder()
        val ids = target.ids.toTypedArray()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            val inline = target.inlineRequest?.let { inlinePresentation(context, it, 0, text, null, R.drawable.ic_autofill_lock) }
            builder.setAuthentication(ids, sender, presentation, inline)
        } else {
            @Suppress("DEPRECATION")
            builder.setAuthentication(ids, sender, presentation)
        }
        return builder.setSaveData(target).build()
    }

    /**
     * Хранилище открыто: подходящие логины со значениями (их получит приложение, только
     * когда пользователь выберет логин) и «Другой логин…» — окно выбора из всех логинов.
     */
    fun logins(context: Context, target: AutofillTarget, requestId: Int, matches: List<LoginMatch>): FillResponse {
        val builder = FillResponse.Builder()
        // Последнее место над клавиатурой — для «Другого логина».
        val inlineSlots = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            target.inlineRequest?.let { maxOf(it.maxSuggestionCount - 1, 0) } ?: 0
        } else {
            0
        }
        matches.forEachIndexed { index, match ->
            val title = match.login.ifEmpty { match.name }
            val dataset = datasetBuilder(context, title, match.name.takeIf { it != title })
            target.usernameId?.let { dataset.setValueCompat(it, AutofillValue.forText(match.login)) }
            target.passwordId?.let { dataset.setValueCompat(it, AutofillValue.forText(match.password)) }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R && index < inlineSlots) {
                target.inlineRequest
                    ?.let { inlinePresentation(context, it, index, title, match.name, R.drawable.ic_autofill_key) }
                    ?.let { dataset.setInlinePresentationCompat(it) }
            }
            builder.addDataset(dataset.build())
        }

        val other = context.getString(if (matches.isEmpty()) R.string.autofill_choose else R.string.autofill_choose_other)
        val picker = Intent(context, AutofillPickerActivity::class.java)
        val dataset = datasetBuilder(context, other, context.getString(R.string.autofill_choose_subtitle), R.drawable.ic_autofill_search)
        target.ids.forEach { dataset.setValueCompat(it, null) }
        dataset.setAuthentication(pendingActivity(context, requestId, target.putInto(picker)).intentSender)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            target.inlineRequest
                ?.let { inlinePresentation(context, it, minOf(matches.size, inlineSlots), other, null, R.drawable.ic_autofill_search) }
                ?.let { dataset.setInlinePresentationCompat(it) }
        }
        builder.addDataset(dataset.build())
        return builder.setSaveData(target).build()
    }

    /** Поля в состоянии ответа (FillResponse.clientState), по которым onSaveRequest найдёт значения. */
    const val STATE_USERNAME_ID = "io.github.f3dosik.shron.autofill.state.USERNAME_ID"
    const val STATE_PASSWORD_ID = "io.github.f3dosik.shron.autofill.state.PASSWORD_ID"

    /**
     * Предложение «Сохранить в Shron?» после входа (SaveInfo) и поля формы для
     * onSaveRequest. Система показывает его, когда форма отправлена или скрылась, а
     * введённое отличается от подставленного из Shron; если подставленный логин изменён,
     * система предлагает обновить его.
     *
     * Экран только с логином — первый шаг входа в два шага: сохранение откладывается
     * (FLAG_DELAY_SAVE, Android 10+) до экрана с паролем, а поле логина переходит в
     * следующий запрос через clientState.
     */
    private fun FillResponse.Builder.setSaveData(target: AutofillTarget): FillResponse.Builder {
        val usernameId = target.usernameId
        val passwordId = target.passwordId
        val info = when {
            passwordId != null -> {
                val type = SaveInfo.SAVE_DATA_TYPE_PASSWORD or
                    (if (usernameId != null) SaveInfo.SAVE_DATA_TYPE_USERNAME else 0)
                SaveInfo.Builder(type, arrayOf(passwordId))
                    .apply { if (usernameId != null) setOptionalIds(arrayOf(usernameId)) }
                    .setFlags(SaveInfo.FLAG_SAVE_ON_ALL_VIEWS_INVISIBLE)
                    .build()
            }
            usernameId != null && Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q ->
                SaveInfo.Builder(SaveInfo.SAVE_DATA_TYPE_USERNAME, arrayOf(usernameId))
                    .setFlags(SaveInfo.FLAG_SAVE_ON_ALL_VIEWS_INVISIBLE or SaveInfo.FLAG_DELAY_SAVE)
                    .build()
            else -> null
        }
        if (info != null) setSaveInfo(info)
        return setClientState(
            Bundle().apply {
                putParcelable(STATE_USERNAME_ID, target.saveUsernameId)
                putParcelable(STATE_PASSWORD_ID, passwordId)
            },
        )
    }

    // Неизменяемый PendingIntent: система не дописывает в него данные, всё нужное уже в
    // наших extras. Свой requestCode на каждый запрос — чтобы extras не перезаписывались.
    private fun pendingActivity(context: Context, requestId: Int, intent: Intent) =
        PendingIntent.getActivity(context, requestId, intent, PendingIntent.FLAG_CANCEL_CURRENT or PendingIntent.FLAG_IMMUTABLE)

    /** Чип над клавиатурой; null — клавиатура не поддерживает нужную версию оформления. */
    // Content.getSlice помечен @RestrictTo, но именно так документация androidx.autofill
    // предлагает строить InlinePresentation.
    @SuppressLint("RestrictedApi")
    @RequiresApi(Build.VERSION_CODES.R)
    private fun inlinePresentation(
        context: Context,
        request: InlineSuggestionsRequest,
        index: Int,
        title: String,
        subtitle: String?,
        @DrawableRes icon: Int,
    ): InlinePresentation? {
        val specs = request.inlinePresentationSpecs
        if (specs.isEmpty()) return null
        val spec: InlinePresentationSpec = specs[minOf(index, specs.size - 1)]
        if (UiVersions.INLINE_UI_VERSION_1 !in UiVersions.getVersions(spec.style)) return null
        // Долгое нажатие на чип открывает Shron.
        val attribution = PendingIntent.getActivity(
            context,
            0,
            Intent(context, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE,
        )
        val content = InlineSuggestionUi.newContentBuilder(attribution)
            .setTitle(title)
            .apply { if (subtitle != null && subtitle != title) setSubtitle(subtitle) }
            .setStartIcon(Icon.createWithResource(context, icon))
            .build()
        return InlinePresentation(content.slice, spec, false)
    }

    // Dataset.Builder.setInlinePresentation устарел в API 33 (Presentations), но работает.
    @RequiresApi(Build.VERSION_CODES.R)
    @Suppress("DEPRECATION")
    private fun Dataset.Builder.setInlinePresentationCompat(presentation: InlinePresentation) =
        setInlinePresentation(presentation)
}
