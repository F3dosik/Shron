<script lang="ts">
  import { api, AppError, errorText } from '../lib/api'
  import KdfWarning from './KdfWarning.svelte'
  import PasswordInput from './PasswordInput.svelte'

  // Разблокировка после автоблокировки, перезапуска или отзыва сессии.
  let {
    login,
    message = '',
    onunlocked,
    onswitch,
    onsettings,
  }: {
    login: string
    message?: string
    onunlocked: () => void
    onswitch: () => void
    onsettings: () => void
  } = $props()

  let password = $state('')
  let error = $state('')
  let kdfWarning = $state('')
  let notice = $state('')
  let busy = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    kdfWarning = ''
    notice = ''
    busy = true
    try {
      await api.unlock(password)
      onunlocked()
    } catch (e) {
      if (e instanceof AppError && e.code === 'KDF_DOWNGRADE') kdfWarning = e.message
      else error = errorText(e)
    } finally {
      busy = false
      password = ''
    }
  }
</script>

<div class="center-screen">
  <form class="card stack" onsubmit={submit}>
    <div>
      <h1>Хранилище заблокировано</h1>
      <p class="lead">{login}</p>
    </div>
    {#if message}<p class="muted" style="margin: 0">{message}</p>{/if}
    <label>
      <span>Мастер-пароль</span>
      <PasswordInput bind:value={password} autofocus />
    </label>
    {#if busy}<p class="muted">Вычисление ключа…</p>{/if}
    {#if error}<p class="error">{error}</p>{/if}
    {#if notice}<p class="muted" style="margin: 0">{notice}</p>{/if}
    {#if kdfWarning}
      <KdfWarning
        {login}
        message={kdfWarning}
        onaccepted={() => { kdfWarning = ''; notice = 'Новые параметры приняты — введите мастер-пароль ещё раз.' }}
      />
    {/if}
    <button type="submit" class="primary" disabled={busy || !password}>Разблокировать</button>
    <div class="row">
      <button type="button" class="link" onclick={onswitch}>Войти в другой аккаунт</button>
      <span class="spacer"></span>
      <button type="button" class="link" onclick={onsettings}>Подключение</button>
    </div>
  </form>
</div>
