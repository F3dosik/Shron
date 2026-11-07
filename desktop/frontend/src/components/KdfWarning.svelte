<script lang="ts">
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert'
  import { api, errorText } from '../lib/api'

  // Предупреждение KDF_DOWNGRADE: сервер прислал параметры Argon2id слабее запомненных.
  // Принять их можно, только если пользователь сам сменил пароль на другом устройстве;
  // после этого вход нужно повторить.
  let {
    login,
    message,
    onaccepted,
  }: {
    login: string
    message: string
    onaccepted: () => void
  } = $props()

  let error = $state('')
  let busy = $state(false)

  async function accept() {
    error = ''
    busy = true
    try {
      await api.acceptKdfChange(login)
      onaccepted()
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<div class="kdf-warning stack">
  <strong class="title"><TriangleAlert size={16} aria-hidden="true" /> Возможная атака на сервер</strong>
  <p>{message}</p>
  <p class="muted">Если вы не меняли пароль, не принимайте параметры и сообщите администратору сервера.</p>
  {#if error}<p class="error">{error}</p>{/if}
  <button type="button" class="danger" disabled={busy} onclick={accept}>
    Я сменил пароль сам — принять параметры
  </button>
</div>

<style>
  .title { display: inline-flex; align-items: center; gap: 6px; }
  .kdf-warning {
    border: 1px solid var(--danger);
    border-radius: var(--radius);
    padding: 12px;
  }
  .kdf-warning p { margin: 0; }
</style>
