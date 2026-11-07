<script lang="ts">
  import { api, errorText } from '../lib/api'
  import PasswordInput from './PasswordInput.svelte'
  import SuggestPassphrase from './SuggestPassphrase.svelte'

  // Замена временного пароля, выданного администратором, на постоянный.
  let {
    login,
    temporaryPassword,
    ondone,
    oncancel,
  }: { login: string; temporaryPassword: string; ondone: () => void; oncancel: () => void } = $props()

  let password = $state('')
  let confirm = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    if (password !== confirm) {
      error = 'Пароли не совпадают'
      return
    }
    busy = true
    try {
      await api.completePasswordChange(temporaryPassword, password)
      ondone()
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<div class="center-screen">
  <form class="card stack" onsubmit={submit}>
    <div>
      <h1>Задайте свой пароль</h1>
      <p class="lead">Аккаунт {login} создан с временным паролем. Придумайте мастер-пароль — администратор его знать не будет.</p>
    </div>
    <label>
      <span>Новый мастер-пароль</span>
      <PasswordInput bind:value={password} autofocus />
    </label>
    <label>
      <span>Повторите</span>
      <PasswordInput bind:value={confirm} />
    </label>
    <SuggestPassphrase onpick={(p) => { password = p; confirm = p }} />
    {#if busy}<p class="muted">Вычисление ключа…</p>{/if}
    {#if error}<p class="error">{error}</p>{/if}
    <div class="row">
      <button type="button" onclick={oncancel}>Отмена</button>
      <span class="spacer"></span>
      <button type="submit" class="primary" disabled={busy || !password}>Сохранить и войти</button>
    </div>
  </form>
</div>
