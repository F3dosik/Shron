<script lang="ts">
  import { api, errorText, type TemporaryUser, type UserRow } from '../lib/api'
  import { formatDate } from '../lib/format'
  import Modal from './Modal.svelte'

  // Административная панель: доступна, когда приложение подключено к административному
  // порту сервера (ADMIN_PORT), то есть фактически только на машине сервера.
  let { serverAddress, onsettings }: { serverAddress: string; onsettings: () => void } = $props()

  let users: UserRow[] = $state([])
  let query = $state('')
  let error = $state('')
  let notice = $state('')
  let loading = $state(true)
  let dialog: '' | 'create' | 'created' | 'revoke' | 'delete' = $state('')
  let target = $state('')
  let newLogin = $state('')
  let typedLogin = $state('')
  let created: TemporaryUser | null = $state(null)
  let busy = $state(false)

  let visible = $derived(users.filter((u) => !query || u.login.toLowerCase().includes(query.toLowerCase())))

  async function reload() {
    error = ''
    loading = true
    try {
      users = await api.adminListUsers()
    } catch (e) {
      error = errorText(e)
    } finally {
      loading = false
    }
  }

  async function run(action: () => Promise<void>, done: string) {
    busy = true
    error = ''
    try {
      await action()
      notice = done
      setTimeout(() => (notice = ''), 4000)
      dialog = ''
      await reload()
    } catch (e) {
      error = errorText(e)
      dialog = ''
    } finally {
      busy = false
    }
  }

  async function createUser(event: SubmitEvent) {
    event.preventDefault()
    busy = true
    error = ''
    try {
      created = await api.adminCreateTemporaryUser(newLogin.trim())
      dialog = 'created'
      newLogin = ''
      await reload()
    } catch (e) {
      error = errorText(e)
      dialog = ''
    } finally {
      busy = false
    }
  }

  function closeCreated() {
    // Временный пароль показывается один раз и не хранится в интерфейсе.
    created = null
    dialog = ''
  }

  $effect(() => {
    reload()
  })
</script>

<div class="admin">
  <header>
    <div>
      <h1>Администрирование</h1>
      <p class="muted">Сервер {serverAddress} · административный порт. Хранилище здесь недоступно.</p>
    </div>
    <span class="spacer"></span>
    <button onclick={reload} disabled={loading}>Обновить</button>
    <button onclick={onsettings}>Подключение</button>
    <button class="primary" onclick={() => (dialog = 'create')}>Создать пользователя</button>
  </header>

  {#if error}<p class="error">{error}</p>{/if}

  <input class="search" placeholder="Поиск по логину" bind:value={query} spellcheck="false" />

  <table>
    <thead>
      <tr><th>Логин</th><th>Создан</th><th>Записей</th><th>Пароль</th><th>Argon2id</th><th></th></tr>
    </thead>
    <tbody>
      {#each visible as u (u.login)}
        <tr>
          <td class="login">{u.login}</td>
          <td>{formatDate(u.createdAt)}</td>
          <td>{u.secretCount}</td>
          <td>
            {#if !u.temporaryUntil}
              постоянный
            {:else if u.temporaryExpired}
              <span class="error">временный, истёк</span>
            {:else}
              <span class="warn">временный до {formatDate(u.temporaryUntil)}</span>
            {/if}
          </td>
          <td class="muted">t={u.kdfTime}, {u.kdfMemoryMiB} MiB</td>
          <td class="actions">
            <button onclick={() => { target = u.login; dialog = 'revoke' }}>Завершить сеансы</button>
            <button class="danger" onclick={() => { target = u.login; typedLogin = ''; dialog = 'delete' }}>Удалить</button>
          </td>
        </tr>
      {:else}
        <tr><td colspan="6" class="muted empty">{loading ? 'Загрузка…' : users.length ? 'Ничего не найдено' : 'Пользователей нет'}</td></tr>
      {/each}
    </tbody>
  </table>
</div>

{#if notice}<div class="notice">{notice}</div>{/if}

{#if dialog === 'create'}
  <Modal title="Новый пользователь" onclose={() => (dialog = '')}>
    <form class="stack" onsubmit={createUser}>
      <p style="margin: 0" class="muted">
        Будет создан временный пароль на ограниченное время. При первом входе пользователь задаст свой пароль,
        и вы его знать не будете.
      </p>
      <label>
        <span>Логин</span>
        <!-- svelte-ignore a11y_autofocus -->
        <input bind:value={newLogin} autocomplete="off" spellcheck="false" autofocus />
      </label>
      <div class="row">
        <span class="spacer"></span>
        <button type="button" onclick={() => (dialog = '')}>Отмена</button>
        <button type="submit" class="primary" disabled={busy || !newLogin.trim()}>Создать</button>
      </div>
    </form>
  </Modal>
{:else if dialog === 'created' && created}
  <Modal title="Пользователь создан" onclose={closeCreated}>
    <div class="stack">
      <p style="margin: 0">Временный пароль для <b>{created.login}</b>:</p>
      <code class="temp">{created.password}</code>
      <p style="margin: 0" class="muted">
        Действует до {formatDate(created.expiresAt)}. Передайте его надёжным каналом — после закрытия окна
        пароль больше не будет показан.
      </p>
      <div class="row">
        <button onclick={() => created && api.copy(created.password)}>Копировать</button>
        <span class="spacer"></span>
        <button class="primary" onclick={closeCreated}>Готово</button>
      </div>
    </div>
  </Modal>
{:else if dialog === 'revoke'}
  <Modal title="Завершить сеансы?" onclose={() => (dialog = '')}>
    <div class="stack">
      <p style="margin: 0">
        На всех устройствах пользователя <b>{target}</b> потребуется снова ввести мастер-пароль.
        Пароль и записи не меняются.
      </p>
      <div class="row">
        <span class="spacer"></span>
        <button onclick={() => (dialog = '')}>Отмена</button>
        <button class="primary" disabled={busy} onclick={() => run(() => api.adminRevokeSessions(target), `Сеансы ${target} завершены`)}>
          Завершить
        </button>
      </div>
    </div>
  </Modal>
{:else if dialog === 'delete'}
  <Modal title="Удалить пользователя?" onclose={() => (dialog = '')}>
    <div class="stack">
      <p style="margin: 0">Пользователь <b>{target}</b> и все его записи будут удалены без возможности восстановления.</p>
      <label><span>Для подтверждения введите логин</span><input bind:value={typedLogin} autocomplete="off" spellcheck="false" /></label>
      <div class="row">
        <span class="spacer"></span>
        <button onclick={() => (dialog = '')}>Отмена</button>
        <button class="danger primary" disabled={busy || typedLogin !== target} onclick={() => run(() => api.adminDeleteUser(target), `${target} удалён`)}>
          Удалить навсегда
        </button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .admin { padding: 24px; height: 100%; overflow: auto; }
  header { display: flex; align-items: center; gap: 8px; margin-bottom: 16px; }
  h1 { margin: 0; font-size: 20px; }
  header p { margin: 4px 0 0; }
  .search { max-width: 320px; margin-bottom: 12px; }
  table { width: 100%; border-collapse: collapse; }
  th { text-align: left; font-weight: normal; color: var(--muted); font-size: 12px; padding: 8px; border-bottom: 1px solid var(--border); }
  td { padding: 8px; border-bottom: 1px solid var(--border); }
  td.login { font-weight: 600; user-select: text; }
  td.actions { text-align: right; white-space: nowrap; }
  td.actions button { padding: 4px 10px; font-size: 12px; }
  td.empty { text-align: center; padding: 24px; }
  .warn { color: #e0b44c; }
  .temp { display: block; padding: 12px; font-size: 18px; text-align: center; background: var(--panel-2); border-radius: 8px; user-select: text; letter-spacing: 1px; }
  .notice { position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%); padding: 10px 16px; background: var(--panel-2); border: 1px solid var(--border); border-radius: var(--radius); }
</style>
