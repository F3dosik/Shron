<script lang="ts">
  import { api, errorText } from '../lib/api'
  import Modal from './Modal.svelte'
  import PasswordInput from './PasswordInput.svelte'
  import SuggestPassphrase from './SuggestPassphrase.svelte'

  let { onclose, ondone }: { onclose: () => void; ondone: () => void } = $props()

  let current = $state('')
  let next = $state('')
  let confirm = $state('')
  let kdfTime = $state(3)
  let kdfMemory = $state(64)
  let advanced = $state(false)
  let error = $state('')
  let busy = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    if (next !== confirm) {
      error = 'Пароли не совпадают'
      return
    }
    busy = true
    try {
      await api.changePassword(current, next, Number(kdfTime), Number(kdfMemory))
      ondone()
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<Modal title="Смена мастер-пароля" {onclose}>
  <form class="stack" onsubmit={submit}>
    <label><span>Текущий пароль</span><PasswordInput bind:value={current} autofocus /></label>
    <label><span>Новый пароль</span><PasswordInput bind:value={next} /></label>
    <label><span>Повторите новый пароль</span><PasswordInput bind:value={confirm} /></label>
    <SuggestPassphrase onpick={(p) => { next = p; confirm = p }} />

    <button type="button" class="link" style="align-self: flex-start" onclick={() => (advanced = !advanced)}>
      {advanced ? '▾' : '▸'} Сложность ключа (Argon2id)
    </button>
    {#if advanced}
      <div class="row">
        <label style="flex: 1"><span>Проходов (1–10)</span><input type="number" min="1" max="10" bind:value={kdfTime} /></label>
        <label style="flex: 1"><span>Память, MiB (19–1024)</span><input type="number" min="19" max="1024" bind:value={kdfMemory} /></label>
      </div>
      <p class="muted" style="margin: 0; font-size: 12px">Больше — дороже перебор пароля при утечке БД, но медленнее разблокировка на всех устройствах.</p>
    {/if}

    <p class="muted" style="margin: 0; font-size: 12px">Все записи будут перешифрованы. На других устройствах потребуется войти заново.</p>
    {#if busy}<p class="muted">Перешифровка…</p>{/if}
    {#if error}<p class="error">{error}</p>{/if}
    <div class="row">
      <span class="spacer"></span>
      <button type="button" onclick={onclose} disabled={busy}>Отмена</button>
      <button type="submit" class="primary" disabled={busy || !current || !next}>Сменить</button>
    </div>
  </form>
</Modal>
