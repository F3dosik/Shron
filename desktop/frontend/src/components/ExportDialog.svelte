<script lang="ts">
  import { api, errorText } from '../lib/api'
  import Modal from './Modal.svelte'
  import PasswordInput from './PasswordInput.svelte'
  import SuggestPassphrase from './SuggestPassphrase.svelte'

  let { onclose, ondone }: { onclose: () => void; ondone: (path: string) => void } = $props()

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
      const path = await api.exportVault(password)
      if (path) ondone(path)
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<Modal title="Экспорт хранилища" {onclose}>
  <form class="stack" onsubmit={submit}>
    <p class="muted" style="margin: 0">
      Все записи будут сохранены в файл, зашифрованный отдельным паролем экспорта. Файл не зависит от сервера:
      его можно хранить где угодно и импортировать в любой аккаунт.
    </p>
    <label><span>Пароль экспорта</span><PasswordInput bind:value={password} autofocus /></label>
    <label><span>Повторите</span><PasswordInput bind:value={confirm} /></label>
    <SuggestPassphrase onpick={(p) => { password = p; confirm = p }} />
    <p class="muted" style="margin: 0; font-size: 12px">Без этого пароля файл не расшифровать — храните его отдельно от файла.</p>
    {#if busy}<p class="muted">Шифрование…</p>{/if}
    {#if error}<p class="error">{error}</p>{/if}
    <div class="row">
      <span class="spacer"></span>
      <button type="button" onclick={onclose} disabled={busy}>Отмена</button>
      <button type="submit" class="primary" disabled={busy || !password}>Сохранить файл…</button>
    </div>
  </form>
</Modal>
