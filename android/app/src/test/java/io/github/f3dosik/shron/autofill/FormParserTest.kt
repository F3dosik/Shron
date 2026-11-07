package io.github.f3dosik.shron.autofill

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class FormParserTest {

    private val textPassword = 0x81 // TYPE_CLASS_TEXT | TYPE_TEXT_VARIATION_PASSWORD
    private val webPassword = 0xe1 // TYPE_CLASS_TEXT | TYPE_TEXT_VARIATION_WEB_PASSWORD
    private val email = 0x21 // TYPE_CLASS_TEXT | TYPE_TEXT_VARIATION_EMAIL_ADDRESS
    private val plainText = 0x01
    private val pin = 0x12 // TYPE_CLASS_NUMBER | TYPE_NUMBER_VARIATION_PASSWORD

    @Test
    fun `autofill hints identify both fields`() {
        val form = parseLoginForm(
            listOf(
                FormField("u", autofillHints = listOf("username")),
                FormField("p", autofillHints = listOf("password")),
            ),
        )
        assertEquals(LoginForm("u", "p"), form)
    }

    @Test
    fun `input types identify both fields`() {
        val form = parseLoginForm(listOf(FormField("u", inputType = email), FormField("p", inputType = textPassword)))
        assertEquals(LoginForm("u", "p"), form)
    }

    @Test
    fun `field right before the password is the login when nothing is marked`() {
        val form = parseLoginForm(
            listOf(
                FormField("search", inputType = plainText),
                FormField("name", inputType = plainText),
                FormField("p", inputType = webPassword),
            ),
        )
        assertEquals(LoginForm("name", "p"), form)
    }

    @Test
    fun `html attributes on web pages`() {
        val form = parseLoginForm(
            listOf(
                FormField("q", html = mapOf("type" to "search", "name" to "q")),
                FormField("u", html = mapOf("type" to "text", "autocomplete" to "username")),
                FormField("p", html = mapOf("type" to "password")),
            ),
        )
        assertEquals(LoginForm("u", "p"), form)
    }

    @Test
    fun `resource names and hints in Russian`() {
        val form = parseLoginForm(
            listOf(
                FormField("u", inputType = plainText, hint = "Логин или телефон"),
                FormField("p", inputType = plainText, idEntry = "et_password"),
            ),
        )
        assertEquals(LoginForm("u", "p"), form)
    }

    @Test
    fun `short words match only as whole words`() {
        assertEquals(false, FormField("x", idEntry = "passport_number").isPassword())
        assertEquals(true, FormField("x", idEntry = "user_pass").isPassword())
        assertEquals(true, FormField("x", idEntry = "userPwd").isPassword())
        assertEquals(false, FormField("x", idEntry = "mailing_address").isUsername())
        assertEquals(true, FormField("x", idEntry = "userName").isUsername())
    }

    @Test
    fun `numeric pin counts as a password`() {
        assertEquals(LoginForm(null, "p"), parseLoginForm(listOf(FormField("p", inputType = pin))))
    }

    @Test
    fun `first step of a two-step sign-in has only the login`() {
        val form = parseLoginForm(listOf(FormField("u", autofillHints = listOf("emailAddress"))))
        assertEquals(LoginForm("u", null), form)
    }

    @Test
    fun `forms without login fields are ignored`() {
        assertNull(parseLoginForm(listOf(FormField("q", inputType = plainText, hint = "Поиск"))))
        assertNull(parseLoginForm(emptyList<FormField<String>>()))
    }

    @Test
    fun `keyword from a web domain`() {
        assertEquals("github", searchKeyword("github.com", null))
        assertEquals("github", searchKeyword("www.github.com", null))
        assertEquals("google", searchKeyword("accounts.google.co.uk", null))
        assertEquals("yandex", searchKeyword("passport.yandex.ru", null))
        assertEquals("localhost", searchKeyword("localhost", null))
    }

    @Test
    fun `keyword from a package when there is no domain`() {
        assertEquals("github", searchKeyword(null, "com.github.android"))
        assertEquals("sberbank", searchKeyword("", "ru.sberbank.android.mobile"))
        assertEquals("", searchKeyword(null, null))
    }

    @Test
    fun `binding uri is the site in a browser and the package in an app`() {
        assertEquals("github.com", bindingUri("www.GitHub.com", "com.android.chrome"))
        assertEquals("androidapp://com.github.android", bindingUri(null, "com.github.android"))
        assertEquals("androidapp://com.github.android", bindingUri(" ", "com.github.android"))
        assertNull(bindingUri(null, null))
    }

    @Test
    fun `uri is added once`() {
        assertEquals("github.com", addUri("", "github.com"))
        assertEquals("a.ru\ngithub.com", addUri("a.ru\n\n", "github.com"))
        assertEquals("GitHub.com\na.ru", addUri("GitHub.com\na.ru", "github.com"))
    }
}
