<script lang="ts">
  import ChevronRight from '@lucide/svelte/icons/chevron-right'
  import Lock from '@lucide/svelte/icons/lock'
  import Plus from '@lucide/svelte/icons/plus'
  import Search from '@lucide/svelte/icons/search'
  import SettingsIcon from '@lucide/svelte/icons/settings'
  import { api, AppError, errorText, lockedCodes, type Secret, type SecretSummary } from '../lib/api'
  import { secretTypes } from '../lib/format'
  import Modal from './Modal.svelte'
  import SecretDetail from './SecretDetail.svelte'
  import SecretForm from './SecretForm.svelte'
  import Settings from './Settings.svelte'
  import TypeIcon from './TypeIcon.svelte'

  let {
    login,
    serverAddress,
    onlocked,
    onsignedout,
    onsettings,
  }: { login: string; serverAddress: string; onlocked: () => void; onsignedout: () => void; onsettings: () => void } = $props()

  let secrets: SecretSummary[] = $state([])
  let query = $state('')
  let typeFilter = $state('')
  let selected: { name: string; type: string } | null = $state(null)
  let mode: 'view' | 'create' | 'edit' | 'settings' = $state('view')
  let editing: Secret | null = $state(null)
  let dialog: '' | 'delete-secret' = $state('')
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

  $effect(() => {
    reload()
  })
</script>

<div class="layout">
  <aside>
    <div class="toolbar">
      <div class="search">
        <Search size={15} aria-hidden="true" />
        <input placeholder="Поиск" bind:value={query} spellcheck="false" />
      </div>
      <button class="primary icon" title="Новая запись" aria-label="Новая запись" onclick={() => { mode = 'create'; selected = null }}>
        <Plus size={18} />
      </button>
    </div>
    <div class="filters">
      <label class="type-filter" class:active={typeFilter !== ''}>
        <span>Тип:</span>
        <select bind:value={typeFilter}>
          <option value="">все</option>
          {#each secretTypes as t}<option value={t.value}>{t.label}</option>{/each}
        </select>
      </label>
    </div>

    <ul>
      {#each sections as section (section.group)}
        {@const open = !showHeaders || searching || !collapsed.has(section.group)}
        {#if showHeaders}
          <li>
            <button class="group" aria-expanded={open} disabled={searching} onclick={() => toggleGroup(section.group)}>
              <span class="chevron" class:open><ChevronRight size={14} /></span>
              <span class="group-name">{section.group || 'Без папки'}</span>
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
              <span class="type-icon"><TypeIcon type={s.type} /></span>
              <span class="names">
                <span class="name">{s.name}</span>
                {#if s.metadata}<span class="meta">{s.metadata}</span>{/if}
              </span>
            </button>
          </li>
        {/each}
      {:else}
        <li class="empty muted">{secrets.length ? 'Ничего не найдено' : 'Записей пока нет'}</li>
      {/each}
    </ul>

    <footer>
      <button class="account" class:active={mode === 'settings'} title="Настройки" onclick={() => { mode = 'settings'; selected = null }}>
        <SettingsIcon size={16} aria-hidden="true" /> <span class="account-name">{login}</span>
      </button>
      <button class="icon" title="Заблокировать" aria-label="Заблокировать" onclick={() => api.lock()}><Lock size={16} /></button>
    </footer>
  </aside>

  <main>
    {#if error}<p class="error banner">{error}</p>{/if}
    {#if mode === 'settings'}
      <Settings
        {login}
        {serverAddress}
        onclose={() => (mode = 'view')}
        onnotice={showNotice}
        onimported={reload}
        {onlocked}
        {onsignedout}
        onconnection={onsettings}
      />
    {:else if mode === 'create'}
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
      <div class="placeholder muted">Выберите запись слева или создайте новую</div>
    {/if}
  </main>
</div>

{#if notice}<div class="notice">{notice}</div>{/if}

{#if dialog === 'delete-secret' && selected}
  <Modal title="Удалить запись?" onclose={() => (dialog = '')}>
    <div class="stack">
      <p style="margin: 0">«{selected.name}» будет удалена без возможности восстановления.</p>
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
  .search { flex: 1; display: flex; align-items: center; gap: 6px; padding: 0 10px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--panel-2); color: var(--muted); }
  .search:focus-within { outline: 2px solid var(--accent); outline-offset: -1px; }
  .search input { border: none; background: none; padding: 8px 0; }
  .search input:focus { outline: none; }
  .icon { display: inline-flex; align-items: center; justify-content: center; padding: 6px 10px; }
  .filters { padding: 0 12px 8px; }
  .type-filter { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); }
  .type-filter select { width: auto; padding: 4px 8px; font-size: 12px; }
  .type-filter.active select { border-color: var(--accent); color: var(--accent); }
  ul { list-style: none; margin: 0; padding: 0 6px; overflow: auto; flex: 1; }
  .type-icon { display: inline-flex; color: var(--muted); }
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
  .chevron { display: inline-flex; transition: transform 0.15s; }
  .chevron.open { transform: rotate(90deg); }
  .group-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .count { font-weight: 400; }
  footer { display: flex; gap: 8px; padding: 12px; border-top: 1px solid var(--border); }
  .account { flex: 1; display: flex; align-items: center; gap: 8px; min-width: 0; text-align: left; }
  .account.active { border-color: var(--accent); color: var(--accent); }
  .account-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .placeholder { display: flex; align-items: center; justify-content: center; height: 100%; }
  .banner { margin: 16px 24px 0; }
  .notice { position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%); padding: 10px 16px; background: var(--panel-2); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4); }
</style>
