package io.github.f3dosik.shron.ui

import androidx.annotation.StringRes
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import io.github.f3dosik.shron.MainActivity
import io.github.f3dosik.shron.R
import io.github.f3dosik.shron.ShronApplication
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Сценарии интерфейса с настоящим Go-ядром без сервера. Каждый тест начинается с чистыми
 * данными приложения (Test Orchestrator с clearPackageData).
 */
@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class ShronAppTest {

    @get:Rule
    val rule = createAndroidComposeRule<MainActivity>()

    private fun str(@StringRes id: Int) = rule.activity.getString(id)

    private fun click(text: String) {
        rule.onNodeWithText(text).performScrollTo().performClick()
    }

    private fun type(label: String, value: String) {
        rule.onNode(hasSetTextAction() and hasText(label)).performScrollTo().performTextInput(value)
    }

    private fun waitForText(text: String, substring: Boolean = false, timeoutMs: Long = 20_000) {
        rule.waitUntil(timeoutMs) {
            rule.onAllNodesWithText(text, substring = substring).fetchSemanticsNodes().isNotEmpty()
        }
    }

    /** Сохраняет адрес, по которому никто не слушает: настройки валидны, сервер недоступен. */
    private fun configureUnreachableServer() {
        type(str(R.string.server_address), "127.0.0.1:1")
        click(str(R.string.save))
        waitForText(str(R.string.sign_in_title))
    }

    @Test
    fun firstLaunchAsksForServerAndValidatesAddress() {
        rule.onNodeWithText(str(R.string.server_address)).assertExists()
        assertTrue(
            "there is nothing to go back to on first launch",
            rule.onAllNodesWithText(str(R.string.cancel)).fetchSemanticsNodes().isEmpty(),
        )

        click(str(R.string.save))
        waitForText("порт необязателен", substring = true)
    }

    @Test
    fun backgroundLockSettingIsSaved() {
        click(str(R.string.background_lock_now))
        configureUnreachableServer()

        val app = rule.activity.application as ShronApplication
        assertEquals(0, app.preferences.backgroundLockSeconds)
    }

    @Test
    fun registrationGeneratesAPassphraseIntoBothFields() {
        configureUnreachableServer()
        click(str(R.string.switch_to_register))
        click(str(R.string.generate_master_password))
        waitForText("Надёжность", substring = true)

        // Два поля пароля и поле логина; пароль показан открытым, чтобы его записать.
        val values = rule.onAllNodes(hasSetTextAction()).fetchSemanticsNodes()
            .mapNotNull { it.config.getOrNull(SemanticsProperties.EditableText)?.text }
            .filter { it.isNotEmpty() }
        assertEquals(values.toString(), 2, values.size)
        assertEquals(values[0], values[1])
        assertEquals(values[0], 6, values[0].split("-").size)
        // Поиск по тексту находит и скрытый пароль, поэтому видимость — по кнопкам «Скрыть».
        rule.onAllNodesWithText(str(R.string.hide_password)).assertCountEquals(2)
    }

    @Test
    fun signInReportsUnreachableServer() {
        configureUnreachableServer()
        type(str(R.string.login), "alice")
        type(str(R.string.master_password), "correct horse")
        click(str(R.string.sign_in))
        waitForText("Сервер недоступен")
    }

    @Test
    fun settingsCanBeReopenedAndCancelled() {
        configureUnreachableServer()
        click(str(R.string.server_settings))
        rule.onNodeWithText("127.0.0.1:1").assertExists()
        click(str(R.string.cancel))
        waitForText(str(R.string.sign_in_title))
    }

}
