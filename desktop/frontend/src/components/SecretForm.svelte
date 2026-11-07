<script lang="ts">
  import { untrack } from 'svelte'
  import { api, AppError, errorText, lockedCodes, type ChosenFile, type Secret } from '../lib/api'
  import { formatSize, secretTypes } from '../lib/format'
  import Generator from './Generator.svelte'
  import PasswordInput from './PasswordInput.svelte'
  import TypeIcon from './TypeIcon.svelte'

  // Создание (initial не задан) или изменение секрета. Имя и тип определяют секрет,
  // поэтому при изменении они не редактируются.
  let {
    initial = null,
    groups = [],
    onsaved,
    oncancel,
    onlocked,
  }: {
    initial?: Secret | null
    groups?: string[]
    onsaved: (name: string, type: string) => void
    oncancel: () => void
    onlocked: () => void
  } = $props()

  // Поля заполняются исходным секретом один раз при открытии формы.
  const start = untrack(() => initial)
  const editing = start !== null
  let name = $state(start?.name ?? '')
  let type = $state(start?.type ?? 'credentials')
  let metadata = $state(start?.metadata ?? '')
  let group = $state(start?.group ?? '')
  let login = $state(start?.login ?? '')
  let password = $state(start?.password ?? '')
  let uris = $state(start?.uris ?? '')
  let text = $state(start?.text ?? '')
  let cardNumber = $state(start?.cardNumber ?? '')
  let cardHolder = $state(start?.cardHolder ?? '')
  let cardExpiry = $state(start?.cardExpiry ?? '')
  let cardCvv = $state(start?.cardCvv ?? '')
  let file: ChosenFile | null = $state(null)
  let error = $state('')
  let busy = $state(false)

  async function chooseFile() {
    try {
      const chosen = await api.chooseFile()
      if (chosen.path) {
        file = chosen
        if (!name) name = chosen.name
      }
    } catch (e) {
      error = errorText(e)
    }
  }

  let showGenerator = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    busy = true
    try {
      await api.saveSecret(
        {
          name, type, metadata, group, login, password, uris, text,
          cardNumber, cardHolder, cardExpiry, cardCvv,
          filePath: file?.path ?? '',
        },
        editing,
      )
      onsaved(name.trim(), type)
    } catch (e) {
      if (e instanceof AppError && lockedCodes.has(e.code)) {
        onlocked()
        return
      }
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<form class="form stack" onsubmit={submit}>
  <h2>{editing ? 'Изменить запись' : 'Новая запись'}</h2>

  {#if !editing}
    <div class="types">
      {#each secretTypes as t}
        <button type="button" class:selected={type === t.value} onclick={() => (type = t.value)}><TypeIcon type={t.value} size={14} /> {t.label}</button>
      {/each}
    </div>
  {/if}

  <label>
    <span>Название</span>
    <!-- svelte-ignore a11y_autofocus -->
    <input bind:value={name} disabled={editing} autocomplete="off" spellcheck="false" autofocus={!editing} />
  </label>

  {#if type === 'credentials'}
    <label><span>Логин</span><input bind:value={login} autocomplete="off" spellcheck="false" /></label>
    <label>
      <span>Пароль</span>
      <PasswordInput bind:value={password} />
    </label>
    {#if showGenerator}
      <Generator onuse={(p) => { password = p; showGenerator = false }} onclose={() => (showGenerator = false)} />
    {:else}
      <div><button type="button" class="link" onclick={() => (showGenerator = true)}>Сгенерировать пароль…</button></div>
    {/if}
    <label>
      <span>Сайты и приложения (необязательно)</span>
      <textarea bind:value={uris} rows="2" placeholder="github.com&#10;androidapp://com.github.android" spellcheck="false"></textarea>
    </label>
    <p class="muted" style="margin: 0; font-size: 12px">По одному на строку. По ним Android-приложение подбирает логин при автозаполнении.</p>
  {:else if type === 'text'}
    <label><span>Текст</span><textarea bind:value={text}></textarea></label>
  {:else if type === 'card'}
    <label><span>Номер карты</span><input bind:value={cardNumber} inputmode="numeric" autocomplete="off" /></label>
    <label><span>Имя владельца</span><input bind:value={cardHolder} autocomplete="off" /></label>
    <div class="row">
      <label style="flex: 1"><span>Срок (ММ/ГГ)</span><input bind:value={cardExpiry} placeholder="12/30" autocomplete="off" /></label>
      <label style="flex: 1"><span>CVV/CVC</span><PasswordInput bind:value={cardCvv} /></label>
    </div>
  {:else if type === 'binary'}
    <div class="row">
      <button type="button" onclick={chooseFile}>{file ? 'Выбрать другой файл…' : 'Выбрать файл…'}</button>
      {#if file}
        <span class="muted">{file.name}, {formatSize(file.size)}</span>
      {:else if editing}
        <span class="muted">оставить текущий файл</span>
      {/if}
    </div>
    <p class="muted" style="margin: 0; font-size: 12px">Сервер принимает файлы примерно до 750 КБ.</p>
  {/if}

  <label>
    <span>Папка (необязательно)</span>
    <input bind:value={group} list="secret-groups" placeholder="Например, Работа" autocomplete="off" spellcheck="false" />
    <datalist id="secret-groups">
      {#each groups as g}<option value={g}></option>{/each}
    </datalist>
  </label>

  <label><span>Комментарий (необязательно)</span><input bind:value={metadata} autocomplete="off" /></label>

  {#if error}<p class="error">{error}</p>{/if}
  <div class="row">
    <span class="spacer"></span>
    <button type="button" onclick={oncancel}>Отмена</button>
    <button type="submit" class="primary" disabled={busy}>Сохранить</button>
  </div>
</form>

<style>
  .form { padding: 24px; max-width: 520px; }
  h2 { margin: 0; font-size: 20px; }
  .types { display: flex; flex-wrap: wrap; gap: 6px; }
  .types button { display: inline-flex; align-items: center; gap: 6px; }
  .types button.selected { border-color: var(--accent); color: var(--accent); }
</style>
