<script lang="ts">
  import { untrack } from 'svelte'
  import { api, errorText, type State } from '../lib/api'

  // Первый запуск и настройки: адрес сервера, CA-сертификат, автоблокировка.
  let {
    appState,
    onsaved,
    oncancel,
  }: { appState: State; onsaved: () => void; oncancel?: () => void } = $props()

  // Поля формы заполняются текущими настройками один раз при открытии.
  const initial = untrack(() => appState)
  let address = $state(initial.serverAddress || '')
  let autoLock = $state(initial.autoLockMinutes ?? 5)
  // Содержимое сохранённого сертификата интерфейсу не передаётся: keepCA означает
  // «использовать сохранённый», caPEM — новый выбранный сертификат.
  let caPEM = $state('')
  let keepCA = $state(initial.hasCaCert)
  let checkResult = $state('')
  let error = $state('')
  let busy = $state(false)

  async function chooseCA() {
    error = ''
    try {
      const pem = await api.chooseCACert()
      if (pem) {
        caPEM = pem
        keepCA = false
      }
    } catch (e) {
      error = errorText(e)
    }
  }

  async function check() {
    error = ''
    checkResult = ''
    busy = true
    try {
      checkResult = await api.checkServer(address, caPEM, keepCA)
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }

  async function save(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    busy = true
    try {
      await api.saveSettings(address, caPEM, keepCA, Number(autoLock))
      onsaved()
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }
</script>

<div class="center-screen">
  <form class="card stack" onsubmit={save}>
    <div>
      <h1>Подключение к серверу</h1>
      <p class="lead">Адрес сервера Shron и его корневой сертификат (ca.crt).</p>
    </div>

    <label>
      <span>Адрес сервера</span>
      <input bind:value={address} placeholder="192.168.1.5 (порт по умолчанию 50051)" autocomplete="off" spellcheck="false" />
    </label>

    <div class="stack" style="gap: 6px">
      <span class="muted" style="font-size: 12px">Корневой сертификат</span>
      <div class="row">
        <button type="button" onclick={chooseCA}>Выбрать ca.crt…</button>
        <span class="muted">
          {#if caPEM}выбран{:else if keepCA}сохранён ранее{:else}системные сертификаты{/if}
        </span>
        <span class="spacer"></span>
        {#if caPEM || keepCA}
          <button type="button" class="link" onclick={() => { caPEM = ''; keepCA = false }}>Убрать</button>
        {/if}
      </div>
    </div>

    <label>
      <span>Блокировать после бездействия, минут (0 — не блокировать)</span>
      <input type="number" min="0" max="1440" bind:value={autoLock} />
    </label>

    {#if checkResult}<p class="ok">{checkResult}</p>{/if}
    {#if error}<p class="error">{error}</p>{/if}

    <div class="row">
      <button type="button" onclick={check} disabled={busy || !address}>Проверить подключение</button>
      <span class="spacer"></span>
      {#if oncancel}<button type="button" onclick={oncancel}>Отмена</button>{/if}
      <button type="submit" class="primary" disabled={busy || !address}>Сохранить</button>
    </div>
  </form>
</div>
