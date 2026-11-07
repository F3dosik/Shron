<script lang="ts">
  import { api, AppError, errorText, lockedCodes, type Secret, type SecretSummary } from '../lib/api'
  import { secretTypes, typeIcon } from '../lib/format'
  import ChangePassword from './ChangePassword.svelte'
  import DeleteAccount from './DeleteAccount.svelte'
  import ExportDialog from './ExportDialog.svelte'
  import ImportDialog from './ImportDialog.svelte'
  import Modal from './Modal.svelte'
  import SecretDetail from './SecretDetail.svelte'
  import SecretForm from './SecretForm.svelte'

  let {
    login,
    onlocked,
    onsignedout,
    onsettings,
  }: { login: string; onlocked: () => void; onsignedout: () => void; onsettings: () => void } = $props()

  let secrets: SecretSummary[] = $state([])
  let query = $state('')
  let typeFilter = $state('')
  let selected: { name: string; type: string } | null = $state(null)
  let mode: 'view' | 'create' | 'edit' = $state('view')
  let editing: Secret | null = $state(null)
  let dialog: '' | 'password' | 'delete-account' | 'delete-secret' | 'menu' | 'export' | 'import' = $state('')
  let notice = $state('')
  let error = $state('')
  let noticeTimer: ReturnType<typeof setTimeout> | undefined

  let visible = $derived(
    secrets
      .filter((s) => !typeFilter || s.type === typeFilter)
      .filter((s) => {
        const q = query.trim().toLowerCase()
        return !q || [s.name, s.metadata, s.group].some((v) => v.toLowerCase().includes(q))
      })
      .sort((a, b) => a.name.localeCompare(b.name, 'ru')),
  )

  // Существующие группы — подсказки в форме секрета.
  let groups = $derived([...new Set(secrets.map((s) => s.group).filter(Boolean))].sort((a, b) => a.localeCompare(b, 'ru')))

  // Видимые секреты по группам: группы по алфавиту, без группы — в конце. Пока групп
  // нет, список остаётся плоским, без заголовков.
  let sections = $derived.by(() => {
    const byGroup = new Map<string, SecretSummary[]>()
    for (const s of visible) {
      const list = byGroup.get(s.group)
      if (list) list.push(s)
      else byGroup.set(s.group, [s])
    }
    return [...byGroup.entries()]
      .sort(([a], [b]) => (a === '' ? 1 : b === '' ? -1 : a.localeCompare(b, 'ru')))
      .map(([group, items]) => ({ group, items }))
  })
  let showHeaders = $derived(groups.length > 0)

  // Свёрнутые группы — только в памяти, пока хранилище открыто. При поиске группы
  // раскрыты, чтобы найденное не пряталось в свёрнутых.
  let collapsed: Set<string> = $state(new Set())
  let searching = $derived(query.trim() !== '')

  function toggleGroup(group: string) {
    const next = new Set(collapsed)
    if (!next.delete(group)) next.add(group)
    collapsed = next
  }

  function handleError(e: unknown) {
    if (e instanceof AppError && lockedCodes.has(e.code)) {
      onlocked()
      return
    }
    error = errorText(e)
  }

  async function reload() {
    error = ''
    try {
      secrets = await api.listSecrets()
    } catch (e) {
      handleError(e)
    }
  }

  function showNotice(text: string) {
    notice = text
    clearTimeout(noticeTimer)
    noticeTimer = setTimeout(() => (notice = ''), 4000)
  }

  async function deleteSelected() {
    if (!selected) return
    try {
      await api.deleteSecret(selected.name, selected.type)
      showNotice(`«${selected.name}» удалён`)
      selected = null
      dialog = ''
      await reload()
    } catch (e) {
      dialog = ''
      handleError(e)
    }
  }

  async function logout(all: boolean) {
    dialog = ''
    try {
      await api.logout(all)
      onsignedout()
    } catch (e) {
      handleError(e)
    }
  }

  $effect(() => {
    reload()
  })
</script>

<div class="layout">
  <aside>
    <div class="toolbar">
      <input class="search" placeholder="Поиск" bind:value={query} spellcheck="false" />
      <button class="primary" title="Новый секрет" onclick={() => { mode = 'create'; selected = null }}>+</button>
    </div>
    <div class="filters">
      <button class:selected={typeFilter === ''} onclick={() => (typeFilter = '')}>Все</button>
      {#each secretTypes as t}
        <button class:selected={typeFilter === t.value} title={t.label} onclick={() => (typeFilter = t.value)}>{t.icon}</button>
      {/each}
    </div>

    <ul>
      {#each sections as section (section.group)}
        {@const open = !showHeaders || searching || !collapsed.has(section.group)}
        {#if showHeaders}
          <li>
            <button class="group" aria-expanded={open} disabled={searching} onclick={() => toggleGroup(section.group)}>
              <span class="chevron" class:open>▸</span>
              <span class="group-name">{section.group || 'Без группы'}</span>
              <span class="count">{section.items.length}</span>
            </button>
          </li>
        {/if}
        {#each open ? section.items : [] as s (s.type + '/' + s.name)}
          <li>
            <button
              class="item"
              class:active={selected?.name === s.name && selected?.type === s.type && mode === 'view'}
              onclick={() => { selected = { name: s.name, type: s.type }; mode = 'view' }}
            >
              <span class="icon">{typeIcon(s.type)}</span>
              <span class="names">
                <span class="name">{s.name}</span>
                {#if s.metadata}<span class="meta">{s.metadata}</span>{/if}
              </span>
            </button>
          </li>
        {/each}
      {:else}
        <li class="empty muted">{secrets.length ? 'Ничего не найдено' : 'Секретов пока нет'}</li>
      {/each}
    </ul>

    <footer>
      <button class="account" onclick={() => (dialog = 'menu')}>👤 {login}</button>
      <button title="Заблокировать" onclick={() => api.lock()}>🔒</button>
    </footer>
  </aside>

  <main>
    {#if error}<p class="error banner">{error}</p>{/if}
    {#if mode === 'create'}
      <SecretForm
        {groups}
        onsaved={async (name, type) => { await reload(); selected = { name, type }; mode = 'view'; showNotice('Сохранено') }}
        oncancel={() => (mode = 'view')}
        {onlocked}
      />
    {:else if mode === 'edit' && editing}
      <SecretForm
        initial={editing}
        {groups}
        onsaved={async (name, type) => { editing = null; await reload(); selected = { name, type }; mode = 'view'; showNotice('Сохранено') }}
        oncancel={() => { editing = null; mode = 'view' }}
        {onlocked}
      />
    {:else if selected}
      <SecretDetail
        name={selected.name}
        type={selected.type}
        onedit={(s) => { editing = s; mode = 'edit' }}
        ondelete={() => (dialog = 'delete-secret')}
        onnotice={showNotice}
        {onlocked}
      />
    {:else}
      <div class="placeholder muted">Выберите секрет слева или создайте новый</div>
    {/if}
  </main>
</div>

{#if notice}<div class="notice">{notice}</div>{/if}

{#if dialog === 'menu'}
  <Modal title={login} onclose={() => (dialog = '')}>
    <div class="stack menu">
      <button onclick={() => (dialog = 'password')}>Сменить мастер-пароль</button>
      <button onclick={() => (dialog = 'export')}>Экспорт хранилища…</button>
      <button onclick={() => (dialog = 'import')}>Импорт из файла…</button>
      <button onclick={() => { dialog = ''; onsettings() }}>Настройки подключения</button>
      <button onclick={() => logout(false)}>Выйти</button>
      <button onclick={() => logout(true)}>Выйти на всех устройствах</button>
      <button class="danger" onclick={() => (dialog = 'delete-account')}>Удалить учётку…</button>
    </div>
  </Modal>
{:else if dialog === 'password'}
  <ChangePassword onclose={() => (dialog = '')} ondone={() => { dialog = ''; showNotice('Пароль изменён') }} />
{:else if dialog === 'export'}
  <ExportDialog onclose={() => (dialog = '')} ondone={(path) => { dialog = ''; showNotice(`Экспорт сохранён: ${path}`) }} />
{:else if dialog === 'import'}
  <ImportDialog onclose={() => (dialog = '')} ondone={() => { dialog = ''; reload() }} />
{:else if dialog === 'delete-account'}
  <DeleteAccount {login} onclose={() => (dialog = '')} ondone={() => { dialog = ''; onsignedout() }} />
{:else if dialog === 'delete-secret' && selected}
  <Modal title="Удалить секрет?" onclose={() => (dialog = '')}>
    <div class="stack">
      <p style="margin: 0">«{selected.name}» будет удалён без возможности восстановления.</p>
      <div class="row">
        <span class="spacer"></span>
        <button onclick={() => (dialog = '')}>Отмена</button>
        <button class="danger primary" onclick={deleteSelected}>Удалить</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .layout { display: flex; height: 100%; }
  aside { width: 300px; min-width: 240px; display: flex; flex-direction: column; background: var(--panel); border-right: 1px solid var(--border); }
  main { flex: 1; overflow: auto; }
  .toolbar { display: flex; gap: 8px; padding: 12px; }
  .toolbar button { font-size: 18px; line-height: 1; padding: 6px 12px; }
  .filters { display: flex; gap: 4px; padding: 0 12px 8px; }
  .filters button { padding: 4px 8px; font-size: 12px; }
  .filters button.selected { border-color: var(--accent); color: var(--accent); }
  ul { list-style: none; margin: 0; padding: 0 6px; overflow: auto; flex: 1; }
  .item { display: flex; align-items: center; gap: 10px; width: 100%; padding: 8px 10px; border: none; border-radius: 6px; background: none; text-align: left; }
  .item:hover { background: var(--panel-2); }
  .item.active { background: var(--panel-2); outline: 1px solid var(--accent); }
  .names { display: flex; flex-direction: column; min-width: 0; }
  .name, .meta { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: 12px; color: var(--muted); }
  .empty { padding: 16px; text-align: center; }
  .group { display: flex; align-items: center; gap: 6px; width: 100%; margin-top: 6px; padding: 6px 10px; border: none; border-radius: 6px; background: none; font-size: 12px; font-weight: 600; color: var(--muted); text-align: left; }
  .group:hover:not(:disabled) { background: var(--panel-2); }
  .group:disabled { cursor: default; opacity: 1; }
  .chevron { display: inline-block; transition: transform 0.15s; }
  .chevron.open { transform: rotate(90deg); }
  .group-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .count { font-weight: 400; }
  footer { display: flex; gap: 8px; padding: 12px; border-top: 1px solid var(--border); }
  .account { flex: 1; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .placeholder { display: flex; align-items: center; justify-content: center; height: 100%; }
  .banner { margin: 16px 24px 0; }
  .notice { position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%); padding: 10px 16px; background: var(--panel-2); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4); }
  .menu button { text-align: left; }
</style>
