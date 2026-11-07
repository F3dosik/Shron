<script lang="ts">
  import X from '@lucide/svelte/icons/x'
  import { api, AppError, errorText, lockedCodes } from '../lib/api'
  import ChangePassword from './ChangePassword.svelte'
  import DeleteAccount from './DeleteAccount.svelte'
  import ExportDialog from './ExportDialog.svelte'
  import ImportDialog from './ImportDialog.svelte'
  import Modal from './Modal.svelte'

  // Настройки открытого хранилища: аккаунт, безопасность, данные, подключение.
  let {
    login,
    serverAddress,
    onclose,
    onnotice,
    onimported,
    onlocked,
    onsignedout,
    onconnection,
  }: {
    login: string
    serverAddress: string
    onclose: () => void
    onnotice: (text: string) => void
    onimported: () => void
    onlocked: () => void
    onsignedout: () => void
    onconnection: () => void
  } = $props()

  let dialog: '' | 'password' | 'export' | 'import' | 'logout-all' | 'delete-account' = $state('')
  let error = $state('')

  async function logout(all: boolean) {
    dialog = ''
    error = ''
    try {
      await api.logout(all)
      onsignedout()
    } catch (e) {
      if (e instanceof AppError && lockedCodes.has(e.code)) onlocked()
      else error = errorText(e)
    }
  }
</script>

<div class="settings">
  <header>
    <h2>Настройки</h2>
    <span class="spacer"></span>
    <button class="icon" title="Закрыть" aria-label="Закрыть" onclick={onclose}><X size={18} /></button>
  </header>

  {#if error}<p class="error">{error}</p>{/if}

  <section>
    <h3>Аккаунт</h3>
    <div class="row-item">
      <div>
        <div>{login}</div>
        <div class="muted small">Сервер: {serverAddress}</div>
      </div>
      <span class="spacer"></span>
      <button onclick={() => logout(false)}>Выйти</button>
    </div>
  </section>

  <section>
    <h3>Безопасность</h3>
    <div class="row-item">
      <div>
        <div>Мастер-пароль</div>
        <div class="muted small">Все записи будут перешифрованы новым паролем.</div>
      </div>
      <span class="spacer"></span>
      <button onclick={() => (dialog = 'password')}>Сменить…</button>
    </div>
    <div class="row-item">
      <div>
        <div>Сеансы</div>
        <div class="muted small">Например, если телефон или ноутбук потерян.</div>
      </div>
      <span class="spacer"></span>
      <button onclick={() => (dialog = 'logout-all')}>Выйти на всех устройствах…</button>
    </div>
  </section>

  <section>
    <h3>Данные</h3>
    <div class="row-item">
      <div>
        <div>Резервная копия</div>
        <div class="muted small">Файл, зашифрованный отдельным паролем; импортируется в любой аккаунт.</div>
      </div>
      <span class="spacer"></span>
      <button onclick={() => (dialog = 'export')}>Экспорт…</button>
      <button onclick={() => (dialog = 'import')}>Импорт…</button>
    </div>
  </section>

  <section>
    <h3>Подключение</h3>
    <div class="row-item">
      <div>
        <div>{serverAddress}</div>
        <div class="muted small">Адрес сервера, сертификат, автоблокировка. После сохранения хранилище заблокируется.</div>
      </div>
      <span class="spacer"></span>
      <button onclick={onconnection}>Изменить…</button>
    </div>
  </section>

  <section class="danger-zone">
    <h3>Удаление аккаунта</h3>
    <div class="row-item">
      <div class="muted small">Аккаунт и все его записи будут удалены с сервера без возможности восстановления.</div>
      <span class="spacer"></span>
      <button class="danger" onclick={() => (dialog = 'delete-account')}>Удалить аккаунт…</button>
    </div>
  </section>
</div>

{#if dialog === 'password'}
  <ChangePassword onclose={() => (dialog = '')} ondone={() => { dialog = ''; onnotice('Мастер-пароль изменён') }} />
{:else if dialog === 'export'}
  <ExportDialog onclose={() => (dialog = '')} ondone={(path) => { dialog = ''; onnotice(`Экспорт сохранён: ${path}`) }} />
{:else if dialog === 'import'}
  <ImportDialog onclose={() => (dialog = '')} ondone={() => { dialog = ''; onimported() }} />
{:else if dialog === 'delete-account'}
  <DeleteAccount {login} onclose={() => (dialog = '')} ondone={() => { dialog = ''; onsignedout() }} />
{:else if dialog === 'logout-all'}
  <Modal title="Выйти на всех устройствах?" onclose={() => (dialog = '')}>
    <div class="stack">
      <p style="margin: 0">Сеансы завершатся на всех устройствах, включая это. Чтобы снова открыть хранилище, понадобится войти с логином и мастер-паролем.</p>
      <div class="row">
        <span class="spacer"></span>
        <button onclick={() => (dialog = '')}>Отмена</button>
        <button class="primary" onclick={() => logout(true)}>Выйти везде</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .settings { padding: 24px; max-width: 640px; }
  header { display: flex; align-items: center; margin-bottom: 8px; }
  h2 { margin: 0; font-size: 20px; }
  h3 { margin: 0 0 8px; font-size: 13px; font-weight: 600; color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em; }
  section { padding: 16px 0; border-top: 1px solid var(--border); }
  section:first-of-type { border-top: none; }
  .row-item { display: flex; align-items: center; gap: 8px; padding: 6px 0; }
  .small { font-size: 12px; }
  .icon { display: inline-flex; padding: 6px; }
  .danger-zone h3 { color: var(--danger); }
</style>
