package io.github.f3dosik.shron.autofill

import io.github.f3dosik.shron.core.LoginMatch
import org.junit.Assert.assertEquals
import org.junit.Test

class SavePlanTest {

    private fun save(username: String, password: String) = PendingSave("github.com", "org.mozilla.firefox", username, password)

    @Test
    fun `new login creates a record bound to the site`() {
        val action = planSave(save("fedos", "secret"), emptyList(), emptyList(), "github.com", "github.com")
        assertEquals(SaveAction.Create("github.com", "fedos", "secret", "github.com"), action)
    }

    @Test
    fun `changed password updates the record with the same login`() {
        val matches = listOf(LoginMatch("GitHub work", "work", "a"), LoginMatch("GitHub", "Fedos", "old"))
        val action = planSave(save("fedos", "new"), matches, listOf("GitHub", "GitHub work"), "github.com", "github.com")
        assertEquals(SaveAction.Update("GitHub", "new"), action)
    }

    @Test
    fun `same password is already saved`() {
        val matches = listOf(LoginMatch("GitHub", "fedos", "secret"))
        val action = planSave(save("fedos", "secret"), matches, listOf("GitHub"), "github.com", "github.com")
        assertEquals(SaveAction.AlreadySaved, action)
    }

    @Test
    fun `other login on the same site creates a new record`() {
        val matches = listOf(LoginMatch("github.com", "fedos", "secret"))
        val action = planSave(save("work", "x"), matches, listOf("github.com"), "github.com", "github.com")
        assertEquals(SaveAction.Create("github.com (work)", "work", "x", "github.com"), action)
    }

    @Test
    fun `password-only form updates the only login of the site`() {
        val matches = listOf(LoginMatch("GitHub", "fedos", "old"))
        val action = planSave(save("", "new"), matches, listOf("GitHub"), "github.com", "github.com")
        assertEquals(SaveAction.Update("GitHub", "new"), action)
    }

    @Test
    fun `password-only form with several logins creates a new record`() {
        val matches = listOf(LoginMatch("a", "a", "1"), LoginMatch("b", "b", "2"))
        val action = planSave(save("", "new"), matches, listOf("a", "b"), "github.com", "github.com")
        assertEquals(SaveAction.Create("github.com", "", "new", "github.com"), action)
    }

    @Test
    fun `unique name skips taken names ignoring case`() {
        assertEquals("site.ru", uniqueName("site.ru", "me", emptyList()))
        assertEquals("site.ru (me)", uniqueName("site.ru", "me", listOf("Site.ru")))
        assertEquals("site.ru (2)", uniqueName("site.ru", "me", listOf("site.ru", "site.ru (me)")))
        assertEquals("site.ru (3)", uniqueName("site.ru", "", listOf("site.ru", "site.ru (2)")))
    }
}
