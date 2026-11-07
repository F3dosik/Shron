package io.github.f3dosik.shron.ui.secrets

import org.junit.Assert.assertEquals
import org.junit.Test

class FilterSecretsTest {

    private val mail = SecretItem("Почта", "credentials", "личный ящик", 0, group = "работа")
    private val bank = SecretItem("Банк", "card", "зарплатная", 0, group = "Банки")
    private val note = SecretItem("Wi-Fi дома", "text", "", 0)
    private val all = listOf(mail, bank, note)

    private fun names(query: String, type: String? = null) = filterSecrets(all, query, type).map { it.name }

    @Test
    fun `empty query and no type keep everything`() {
        assertEquals(listOf("Почта", "Банк", "Wi-Fi дома"), names(""))
    }

    @Test
    fun `query matches name ignoring case`() {
        assertEquals(listOf("Почта"), names("почт"))
        assertEquals(listOf("Wi-Fi дома"), names("WI-FI"))
    }

    @Test
    fun `query matches the comment`() {
        assertEquals(listOf("Банк"), names("Зарплат"))
    }

    @Test
    fun `query matches the group`() {
        assertEquals(listOf("Почта"), names("РАБОТ"))
    }

    @Test
    fun `groups are sorted by name with ungrouped secrets last`() {
        val groups = groupSecrets(listOf(note, mail, bank))
        assertEquals(listOf("Банки", "работа", ""), groups.map { it.name })
        assertEquals(listOf(listOf("Банк"), listOf("Почта"), listOf("Wi-Fi дома")), groups.map { g -> g.items.map { it.name } })
    }

    @Test
    fun `group names are distinct and skip ungrouped`() {
        val second = SecretItem("Пропуск", "text", "", 0, group = "работа")
        assertEquals(listOf("Банки", "работа"), groupNames(listOf(note, mail, second, bank)))
    }

    @Test
    fun `surrounding spaces are ignored`() {
        assertEquals(listOf("Банк"), names("  банк "))
    }

    @Test
    fun `type filter combines with the query`() {
        assertEquals(listOf("Банк"), names("", "card"))
        assertEquals(emptyList<String>(), names("почта", "card"))
    }
}
