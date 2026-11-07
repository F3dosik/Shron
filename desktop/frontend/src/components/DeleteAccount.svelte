<script lang="ts">
  import { api, errorText } from '../lib/api'
  import Modal from './Modal.svelte'
  import PasswordInput from './PasswordInput.svelte'

  let { login, onclose, ondone }: { login: string; onclose: () => void; ondone: () => void } = $props()

  let typedLogin = $state('')
  let password = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    busy = true
    try {
      await api.deleteAccount(password)
      ondone()
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<Modal title="Удаление аккаунта" {onclose}>
  <form class="stack" onsubmit={submit}>
    <p style="margin: 0">Аккаунт <b>{login}</b> и все его записи будут удалены без возможности восстановления.</p>
    <label><span>Для подтверждения введите логин</span><input bind:value={typedLogin} autocomplete="off" spellcheck="false" /></label>
    <label><span>Мастер-пароль</span><PasswordInput bind:value={password} /></label>
    {#if error}<p class="error">{error}</p>{/if}
    <div class="row">
      <span class="spacer"></span>
      <button type="button" onclick={onclose} disabled={busy}>Отмена</button>
      <button type="submit" class="danger primary" disabled={busy || typedLogin !== login || !password}>Удалить навсегда</button>
    </div>
  </form>
</Modal>
