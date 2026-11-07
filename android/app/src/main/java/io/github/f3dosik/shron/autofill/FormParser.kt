package io.github.f3dosik.shron.autofill

/**
 * Поле формы из структуры экрана, которую Android передаёт службе автозаполнения.
 * [id] — AutofillId; в тестах — любая метка. Остальное — признаки, по которым
 * поле узнаётся как логин или пароль.
 */
data class FormField<T>(
    val id: T,
    /** View.getAutofillHints(): «password», «username», «emailAddress»… */
    val autofillHints: List<String> = emptyList(),
    /** InputType поля (android.text.InputType). */
    val inputType: Int = 0,
    /** Имя ресурса id: «login_input», «password»… */
    val idEntry: String? = null,
    /** Подсказка в поле (hint). */
    val hint: String? = null,
    /** Атрибуты HTML-элемента на сайтах: type, name, autocomplete, id. */
    val html: Map<String, String> = emptyMap(),
)

/** Найденные поля входа: хотя бы одно из двух. */
data class LoginForm<T>(val username: T?, val password: T?)

private const val TYPE_MASK_CLASS = 0x0000000f
private const val TYPE_MASK_VARIATION = 0x00000ff0
private const val TYPE_CLASS_TEXT = 0x00000001
private const val TYPE_CLASS_NUMBER = 0x00000002
private const val TYPE_TEXT_VARIATION_EMAIL_ADDRESS = 0x00000020
private const val TYPE_TEXT_VARIATION_PASSWORD = 0x00000080
private const val TYPE_TEXT_VARIATION_VISIBLE_PASSWORD = 0x00000090
private const val TYPE_TEXT_VARIATION_WEB_EMAIL_ADDRESS = 0x000000d0
private const val TYPE_TEXT_VARIATION_WEB_PASSWORD = 0x000000e0
private const val TYPE_NUMBER_VARIATION_PASSWORD = 0x00000010

private val PASSWORD_HINTS = setOf("password", "current-password", "new-password")
private val USERNAME_HINTS = setOf("username", "emailaddress", "email", "phone", "tel")
// Длинные слова ищутся как подстрока («loginInput», «user_email»), короткие — только как
// отдельное слово, чтобы «pass» не находилось в «passport», а «mail» — в «mailing_address».
private val USERNAME_SUBSTRINGS = listOf("login", "username", "email", "e-mail", "phone", "логин", "почт", "телефон")
private val USERNAME_TOKENS = setOf("user", "mail", "tel")
private val PASSWORD_SUBSTRINGS = listOf("password", "passwd", "пароль")
private val PASSWORD_TOKENS = setOf("pass", "pwd", "pin")
private val TOKEN_SEPARATOR = Regex("[^\\p{L}\\p{N}]+|(?<=\\p{Ll})(?=\\p{Lu})")

private fun <T> FormField<T>.texts() = listOfNotNull(idEntry, hint, html["name"], html["id"])

private fun List<String>.matches(substrings: List<String>, tokens: Set<String>) = any { text ->
    val lower = text.lowercase()
    substrings.any { it in lower } || text.split(TOKEN_SEPARATOR).any { it.lowercase() in tokens }
}

private fun <T> FormField<T>.hints() =
    autofillHints.map { it.lowercase() } + listOfNotNull(html["autocomplete"]?.lowercase())

fun <T> FormField<T>.isPassword(): Boolean {
    if (hints().any { it in PASSWORD_HINTS }) return true
    if (html["type"]?.lowercase() == "password") return true
    val cls = inputType and TYPE_MASK_CLASS
    val variation = inputType and TYPE_MASK_VARIATION
    if (cls == TYPE_CLASS_TEXT && variation in setOf(
            TYPE_TEXT_VARIATION_PASSWORD,
            TYPE_TEXT_VARIATION_VISIBLE_PASSWORD,
            TYPE_TEXT_VARIATION_WEB_PASSWORD,
        )
    ) return true
    if (cls == TYPE_CLASS_NUMBER && variation == TYPE_NUMBER_VARIATION_PASSWORD) return true
    return texts().matches(PASSWORD_SUBSTRINGS, PASSWORD_TOKENS)
}

fun <T> FormField<T>.isUsername(): Boolean {
    if (isPassword()) return false
    if (hints().any { it in USERNAME_HINTS }) return true
    if (html["type"]?.lowercase() == "email") return true
    val cls = inputType and TYPE_MASK_CLASS
    val variation = inputType and TYPE_MASK_VARIATION
    if (cls == TYPE_CLASS_TEXT &&
        variation in setOf(TYPE_TEXT_VARIATION_EMAIL_ADDRESS, TYPE_TEXT_VARIATION_WEB_EMAIL_ADDRESS)
    ) return true
    return texts().matches(USERNAME_SUBSTRINGS, USERNAME_TOKENS)
}

/**
 * Находит поля логина и пароля среди полей экрана в порядке их следования. Логин — поле,
 * явно помеченное как логин, иначе ближайшее текстовое поле перед паролем. Форма без поля
 * пароля и без явного поля логина — не форма входа (поиск, адрес, комментарий).
 */
fun <T> parseLoginForm(fields: List<FormField<T>>): LoginForm<T>? {
    val passwordIndex = fields.indexOfFirst { it.isPassword() }
    val explicitUsername = fields.firstOrNull { it.isUsername() }
    if (passwordIndex < 0) {
        return explicitUsername?.let { LoginForm(it.id, null) }
    }
    val username = explicitUsername
        ?: fields.subList(0, passwordIndex).lastOrNull { !it.isPassword() }
    return LoginForm(username?.id, fields[passwordIndex].id)
}

private val COMMON_PACKAGE_PARTS = setOf(
    "com", "org", "net", "io", "ru", "app", "apps", "android", "mobile", "client", "www", "m",
)

/**
 * Слово для поиска секрета по сайту или приложению: у секрета нет поля адреса, поэтому
 * совпадение ищется в названии и комментарии. «www.github.com» → «github»,
 * «accounts.google.co.uk» → «google», «com.github.android» → «github».
 */
fun searchKeyword(webDomain: String?, packageName: String?): String {
    if (!webDomain.isNullOrBlank()) {
        val labels = webDomain.lowercase().trim('.').split('.').filter { it.isNotEmpty() }
        return when {
            labels.size >= 3 && labels[labels.size - 2].length <= 3 -> labels[labels.size - 3]
            labels.size >= 2 -> labels[labels.size - 2]
            else -> labels.firstOrNull().orEmpty()
        }
    }
    if (!packageName.isNullOrBlank()) {
        return packageName.lowercase().split('.').firstOrNull { it.isNotEmpty() && it !in COMMON_PACKAGE_PARTS }
            .orEmpty()
    }
    return ""
}

/**
 * Адрес, который привязывается к логину, выбранному для формы: домен сайта без «www.» или
 * «androidapp://пакет» для приложения. В браузере привязывается сайт, а не сам браузер.
 */
fun bindingUri(webDomain: String?, packageName: String?): String? = when {
    !webDomain.isNullOrBlank() -> webDomain.trim().lowercase().removePrefix("www.").trimEnd('.')
    !packageName.isNullOrBlank() -> "androidapp://" + packageName.trim()
    else -> null
}

/** Добавляет [uri] к сайтам логина (по одному на строку), если его там ещё нет. */
fun addUri(uris: String, uri: String): String {
    val lines = uris.lines().map(String::trim).filter(String::isNotEmpty)
    return if (lines.any { it.equals(uri, ignoreCase = true) }) lines.joinToString("\n") else (lines + uri).joinToString("\n")
}
