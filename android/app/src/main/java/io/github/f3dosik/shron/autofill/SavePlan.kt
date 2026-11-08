package io.github.f3dosik.shron.autofill

import io.github.f3dosik.shron.core.LoginMatch

/**
 * Логин, который пользователь ввёл в форму и согласился сохранить в Shron: сайт (в
 * браузере) или приложение, логин и пароль.
 */
data class PendingSave(
    val webDomain: String?,
    val packageName: String,
    val username: String,
    val password: String,
) {
    override fun toString() = "PendingSave(webDomain=$webDomain, packageName=$packageName)"
}

/** Что сделать с введённым логином. */
sealed interface SaveAction {
    /** Новая запись-логин [name], привязанная к сайту или приложению [uri]. */
    data class Create(val name: String, val login: String, val password: String, val uri: String?) : SaveAction

    /** Сменить пароль у записи [name]: пользователь вошёл с другим паролем. */
    data class Update(val name: String, val password: String) : SaveAction

    /** Такой логин с таким паролем уже есть. */
    data object AlreadySaved : SaveAction
}

/**
 * Решает, создать запись или обновить пароль в существующей. [matches] — логины,
 * привязанные к этому сайту или приложению; [existingNames] — названия всех записей
 * (название должно быть уникальным); [title] — название для новой записи.
 *
 * Запись того же сайта с тем же логином (без учёта регистра) обновляется. Форма без поля
 * логина (только пароль) обновляет единственный логин сайта; если их несколько или нет —
 * создаётся новая запись.
 */
fun planSave(
    save: PendingSave,
    matches: List<LoginMatch>,
    existingNames: Collection<String>,
    title: String,
    uri: String?,
): SaveAction {
    val username = save.username.trim()
    val same = if (username.isEmpty()) {
        matches.singleOrNull()
    } else {
        matches.firstOrNull { it.login.trim().equals(username, ignoreCase = true) }
    }
    if (same != null) {
        return if (same.password == save.password) SaveAction.AlreadySaved else SaveAction.Update(same.name, save.password)
    }
    return SaveAction.Create(uniqueName(title, username, existingNames), username, save.password, uri)
}

/**
 * Свободное название для новой записи: «github.com», занято — «github.com (логин)»,
 * затем «github.com (2)», «github.com (3)»…
 */
fun uniqueName(title: String, username: String, existingNames: Collection<String>): String {
    val taken = existingNames.mapTo(HashSet()) { it.trim().lowercase() }
    val base = title.trim().ifEmpty { username.ifEmpty { "Логин" } }
    val candidates = sequenceOf(base) +
        (if (username.isNotEmpty() && username != base) sequenceOf("$base ($username)") else emptySequence()) +
        generateSequence(2) { it + 1 }.map { "$base ($it)" }
    return candidates.first { it.lowercase() !in taken }
}
