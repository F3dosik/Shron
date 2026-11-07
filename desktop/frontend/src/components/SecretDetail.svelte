<script lang="ts">
  import { api, AppError, errorText, lockedCodes, type Secret } from '../lib/api'
  import Folder from '@lucide/svelte/icons/folder'
  import { formatCardNumber, formatDate, formatSize, maskCardNumber, typeLabel } from '../lib/format'
  import TypeIcon from './TypeIcon.svelte'

  // Карточка секрета. Содержимое запрашивается только при открытии и живёт,
  // пока карточка открыта; значения скрыты, пока их не показали явно.
  let {
    name,
    type,
    onedit,
    ondelete,
    onnotice,
    onlocked,
  }: {
    name: string
    type: string
    onedit: (secret: Secret) => void
    ondelete: () => void
    onnotice: (text: string) => void
    onlocked: () => void
  } = $props()

  let secret: Secret | null = $state(null)
  let error = $state('')
  let revealed: Record<string, boolean> = $state({})

  $effect(() => {
    const current = { name, type }
    secret = null
    error = ''
    revealed = {}
    api
      .getSecret(current.name, current.type)
      .then((s) => {
        if (current.name === name && current.type === type) secret = s
      })
      .catch(handleError)
  })

  function handleError(e: unknown) {
    if (e instanceof AppError && lockedCodes.has(e.code)) {
      onlocked()
      return
    }
    error = errorText(e)
  }

  async function copy(value: string, what: string) {
    try {
      await api.copy(value)
      onnotice(`${what} скопирован — буфер очистится через 30 секунд`)
    } catch (e) {
      handleError(e)
    }
  }

  async function exportFile() {
    try {
      const path = await api.exportFile(name)
      if (path) onnotice(`Файл сохранён: ${path}`)
    } catch (e) {
      handleError(e)
    }
  }

  function toggle(field: string) {
    revealed = { ...revealed, [field]: !revealed[field] }
  }
</script>

{#snippet field(label: string, value: string, key: string, secretValue = false, display = value)}
  {#if value}
    <div class="field">
      <span class="label">{label}</span>
      <div class="value-row">
        <code class:masked={secretValue && !revealed[key]}>
          {secretValue && !revealed[key] ? '••••••••••' : display}
        </code>
        {#if secretValue}
          <button class="link" onclick={() => toggle(key)}>{revealed[key] ? 'Скрыть' : 'Показать'}</button>
        {/if}
        <button class="link" onclick={() => copy(value, label)}>Копировать</button>
      </div>
    </div>
  {/if}
{/snippet}

<div class="detail">
  {#if error}
    <p class="error">{error}</p>
  {:else if !secret}
    <p class="muted">Загрузка…</p>
  {:else}
    <header>
      <div>
        <div class="type muted">
          <TypeIcon type={secret.type} size={13} /> {typeLabel(secret.type)}
          {#if secret.group}<span class="sep">·</span><Folder size={13} aria-hidden="true" /> {secret.group}{/if}
        </div>
        <h2>{secret.name}</h2>
      </div>
      <span class="spacer"></span>
      <button onclick={() => secret && onedit(secret)}>Изменить</button>
      <button class="danger" onclick={ondelete}>Удалить</button>
    </header>

    <div class="fields">
      {#if secret.type === 'credentials'}
        {@render field('Логин', secret.login, 'login')}
        {@render field('Пароль', secret.password, 'password', true)}
        {#if secret.uris}
          <div class="field">
            <span class="label">Сайты и приложения</span>
            <div class="uris">{secret.uris}</div>
          </div>
        {/if}
      {:else if secret.type === 'text'}
        <div class="field">
          <span class="label">Текст</span>
          <pre>{secret.text}</pre>
          <div><button class="link" onclick={() => secret && copy(secret.text, 'Текст')}>Копировать</button></div>
        </div>
      {:else if secret.type === 'card'}
        {@render field('Номер', secret.cardNumber, 'number', false, revealed.number ? formatCardNumber(secret.cardNumber) : maskCardNumber(secret.cardNumber))}
        <div><button class="link" onclick={() => toggle('number')}>{revealed.number ? 'Скрыть номер' : 'Показать номер'}</button></div>
        {@render field('Имя владельца', secret.cardHolder, 'holder')}
        {@render field('Срок действия', secret.cardExpiry, 'expiry')}
        {@render field('CVV/CVC', secret.cardCvv, 'cvv', true)}
      {:else if secret.type === 'binary'}
        <div class="field">
          <span class="label">Файл</span>
          <div class="value-row">
            <code>{formatSize(secret.fileSize)}</code>
            <button class="link" onclick={exportFile}>Сохранить на диск…</button>
          </div>
        </div>
      {/if}

      {#if secret.metadata}
        <div class="field">
          <span class="label">Комментарий</span>
          <div class="plain">{secret.metadata}</div>
        </div>
      {/if}

      <p class="muted dates">
        Создано {formatDate(secret.createdAt)} · изменено {formatDate(secret.updatedAt)}
      </p>
    </div>
  {/if}
</div>

<style>
  .detail { padding: 24px; }
  header { display: flex; align-items: center; gap: 8px; margin-bottom: 20px; }
  h2 { margin: 2px 0 0; font-size: 20px; word-break: break-word; }
  .type { display: flex; align-items: center; gap: 4px; font-size: 12px; }
  .sep { margin: 0 2px; }
  .fields { display: flex; flex-direction: column; gap: 16px; }
  .field { display: flex; flex-direction: column; gap: 4px; }
  .label { font-size: 12px; color: var(--muted); }
  .uris { white-space: pre-line; word-break: break-all; }
  .value-row { display: flex; align-items: center; gap: 8px; }
  code { padding: 6px 10px; background: var(--panel-2); border-radius: 6px; font-size: 14px; user-select: text; word-break: break-all; }
  code.masked { letter-spacing: 2px; color: var(--muted); }
  pre { margin: 0; padding: 10px; background: var(--panel-2); border-radius: 6px; white-space: pre-wrap; word-break: break-word; user-select: text; font: inherit; }
  .plain { user-select: text; white-space: pre-wrap; }
  .dates { font-size: 12px; margin-top: 8px; }
</style>
