<script lang="ts">
  import { api, errorText, type ChosenFile, type ImportResult } from '../lib/api'
  import { formatDate, typeLabel } from '../lib/format'
  import Modal from './Modal.svelte'
  import PasswordInput from './PasswordInput.svelte'

  let { onclose, ondone }: { onclose: () => void; ondone: () => void } = $props()

  let file: ChosenFile | null = $state(null)
  let password = $state('')
  let overwrite = $state(false)
  let result = $state<ImportResult | null>(null)
  let error = $state('')
  let busy = $state(false)

  async function choose() {
    error = ''
    try {
      const chosen = await api.chooseExportFile()
      if (chosen.path) file = chosen
    } catch (e) {
      error = errorText(e)
    }
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    if (!file) return
    error = ''
    busy = true
    try {
      result = await api.importVault(file.path, password, overwrite)
      password = ''
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }

  function close() {
    if (result) ondone()
    else onclose()
  }
</script>

<Modal title="Импорт записей" onclose={close}>
  {#if result}
    <div class="stack">
      <p style="margin: 0">
        Экспорт аккаунта <b>{result.sourceLogin}</b> от {formatDate(result.exportedAt)}, записей в файле: {result.total}.
      </p>
      <ul class="report">
        <li>Создано: <b>{result.created}</b></li>
        <li>Обновлено: <b>{result.updated}</b></li>
        <li>Пропущено (уже есть): <b>{result.skipped}</b></li>
        {#if result.failed.length}<li class="error">Ошибок: {result.failed.length}</li>{/if}
      </ul>
      {#if result.interrupted}
        <p class="error" style="margin: 0">Импорт прерван: {result.interruptReason}. Остальные записи не обработаны.</p>
      {/if}
      {#if result.failed.length}
        <ul class="failures">
          {#each result.failed as f}<li><b>{f.name}</b> ({typeLabel(f.type)}): {f.error}</li>{/each}
        </ul>
      {/if}
      <div class="row"><span class="spacer"></span><button class="primary" onclick={close}>Готово</button></div>
    </div>
  {:else}
    <form class="stack" onsubmit={submit}>
      <div class="row">
        <button type="button" onclick={choose}>{file ? 'Другой файл…' : 'Выбрать файл…'}</button>
        {#if file}<span class="muted">{file.name}</span>{/if}
      </div>
      <label><span>Пароль экспорта</span><PasswordInput bind:value={password} /></label>
      <label class="check"><input type="checkbox" bind:checked={overwrite} /> Перезаписывать записи с тем же названием и типом</label>
      <p class="muted" style="margin: 0; font-size: 12px">Без этой отметки такие записи пропускаются.</p>
      {#if busy}<p class="muted">Импорт…</p>{/if}
      {#if error}<p class="error">{error}</p>{/if}
      <div class="row">
        <span class="spacer"></span>
        <button type="button" onclick={onclose} disabled={busy}>Отмена</button>
        <button type="submit" class="primary" disabled={busy || !file || !password}>Импортировать</button>
      </div>
    </form>
  {/if}
</Modal>

<style>
  .report { margin: 0; padding-left: 18px; }
  .failures { margin: 0; padding-left: 18px; font-size: 12px; max-height: 160px; overflow: auto; }
  .check { flex-direction: row; align-items: center; gap: 8px; color: var(--text); }
  .check input { width: auto; }
</style>
