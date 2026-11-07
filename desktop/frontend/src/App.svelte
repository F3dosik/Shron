<script lang="ts">
  import { onMount } from 'svelte'
  import { EventsOn } from '../wailsjs/runtime/runtime.js'
  import { api, errorText, type State } from './lib/api'
  import Setup from './components/Setup.svelte'
  import SignIn from './components/SignIn.svelte'
  import TemporaryPassword from './components/TemporaryPassword.svelte'
  import Unlock from './components/Unlock.svelte'
  import Vault from './components/Vault.svelte'
  import AdminPanel from './components/AdminPanel.svelte'

  // Экран выбирается по состоянию бэкенда: не настроено → настройки, нет аккаунта →
  // вход, заблокировано → разблокировка, иначе — хранилище.
  let appState: State | null = $state(null)
  let showSettings = $state(false)
  let pendingChange: { login: string; password: string } | null = $state(null)
  let lockMessage = $state('')
  let fatal = $state('')

  const lockMessages: Record<string, string> = {
    idle: 'Хранилище заблокировано из-за бездействия.',
    session_expired: 'Сеанс завершён: на другом устройстве выполнен выход или сменён пароль. Введите мастер-пароль.',
    manual: '',
    signed_out: '',
  }

  async function refresh() {
    try {
      appState = await api.getState()
    } catch (e) {
      fatal = errorText(e)
    }
  }

  onMount(() => {
    refresh()
    const off = EventsOn('vault:locked', (reason: string) => {
      lockMessage = lockMessages[reason] ?? ''
      refresh()
    })
    // Свёрнутое окно блокирует хранилище.
    const onVisibility = () => {
      if (document.hidden && appState?.unlocked) api.lock()
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      off()
      document.removeEventListener('visibilitychange', onVisibility)
    }
  })
</script>

{#if fatal}
  <div class="center-screen"><p class="error">{fatal}</p></div>
{:else if !appState}
  <div class="center-screen muted">Загрузка…</div>
{:else if !appState.configured || showSettings}
  <Setup
    {appState}
    onsaved={() => { showSettings = false; refresh() }}
    oncancel={appState.configured ? () => (showSettings = false) : undefined}
  />
{:else if appState.adminMode}
  <AdminPanel serverAddress={appState.serverAddress} onsettings={() => (showSettings = true)} />
{:else if pendingChange}
  <TemporaryPassword
    login={pendingChange.login}
    temporaryPassword={pendingChange.password}
    ondone={() => { pendingChange = null; refresh() }}
    oncancel={() => { pendingChange = null; refresh() }}
  />
{:else if !appState.login}
  <SignIn
    serverAddress={appState.serverAddress}
    onsignedin={(login, password, changeRequired) => {
      if (changeRequired) pendingChange = { login, password }
      lockMessage = ''
      refresh()
    }}
    onsettings={() => (showSettings = true)}
  />
{:else if !appState.unlocked}
  <Unlock
    login={appState.login}
    message={lockMessage}
    onunlocked={() => { lockMessage = ''; refresh() }}
    onswitch={async () => { await api.logout(false).catch(() => {}); refresh() }}
    onsettings={() => (showSettings = true)}
  />
{:else}
  <Vault
    login={appState.login}
    serverAddress={appState.serverAddress}
    onlocked={refresh}
    onsignedout={() => { lockMessage = ''; refresh() }}
    onsettings={() => (showSettings = true)}
  />
{/if}
