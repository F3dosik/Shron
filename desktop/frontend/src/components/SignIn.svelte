<script lang="ts">
  import { api, AppError, errorText } from '../lib/api'
  import KdfWarning from './KdfWarning.svelte'
  import PasswordInput from './PasswordInput.svelte'
  import SuggestPassphrase from './SuggestPassphrase.svelte'

  // Вход или регистрация. После входа с временным паролем вызывает
  // onsignedin с changeRequired = true: нужно задать постоянный пароль.
  let {
    serverAddress,
    onsignedin,
    onsettings,
  }: {
    serverAddress: string
    onsignedin: (login: string, password: string, changeRequired: boolean) => void
    onsettings: () => void
  } = $props()

  let mode: 'signin' | 'register' = $state('signin')
  let login = $state('')
  let password = $state('')
  let confirm = $state('')
  let error = $state('')
  let kdfWarning = $state('')
  let notice = $state('')
  let busy = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    kdfWarning = ''
    notice = ''
    if (mode === 'register' && password !== confirm) {
      error = 'Пароли не совпадают'
      return
    }
    busy = true
    try {
      if (mode === 'register') {
        await api.register(login, password)
      }
      const changeRequired = await api.signIn(login, password)
      onsignedin(login.trim(), password, changeRequired)
    } catch (e) {
      if (e instanceof AppError && e.code === 'KDF_DOWNGRADE') kdfWarning = e.message
      else error = errorText(e)
    } finally {
      busy = false
      password = ''
      confirm = ''
    }
  }
</script>

<div class="center-screen">
  <form class="card stack" onsubmit={submit}>
    <div>
      <h1>{mode === 'signin' ? 'Вход' : 'Регистрация'}</h1>
      <p class="lead">Сервер {serverAddress}</p>
    </div>

    <label>
      <span>Логин</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input bind:value={login} autocomplete="off" spellcheck="false" autofocus />
    </label>
    <label>
      <span>Мастер-пароль</span>
      <PasswordInput bind:value={password} />
    </label>
    {#if mode === 'register'}
      <label>
        <span>Повторите мастер-пароль</span>
        <PasswordInput bind:value={confirm} />
      </label>
      <SuggestPassphrase onpick={(p) => { password = p; confirm = p }} />
      <p class="muted" style="margin: 0">
        Мастер-пароль нельзя восстановить: записи шифруются ключом из него, и сервер его не знает.
      </p>
    {/if}

    {#if busy}<p class="muted">Вычисление ключа…</p>{/if}
    {#if error}<p class="error">{error}</p>{/if}
    {#if notice}<p class="muted" style="margin: 0">{notice}</p>{/if}
    {#if kdfWarning}
      <KdfWarning
        login={login.trim()}
        message={kdfWarning}
        onaccepted={() => { kdfWarning = ''; notice = 'Новые параметры приняты — введите мастер-пароль ещё раз.' }}
      />
    {/if}

    <button type="submit" class="primary" disabled={busy || !login || !password}>
      {mode === 'signin' ? 'Войти' : 'Зарегистрироваться'}
    </button>
    <div class="row">
      <button type="button" class="link" onclick={() => { mode = mode === 'signin' ? 'register' : 'signin'; error = '' }}>
        {mode === 'signin' ? 'Создать аккаунт' : 'У меня уже есть аккаунт'}
      </button>
      <span class="spacer"></span>
      <button type="button" class="link" onclick={onsettings}>Подключение</button>
    </div>
  </form>
</div>
