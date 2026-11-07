<script lang="ts">
  import { api, errorText, type Generated } from '../lib/api'

  // Генератор пароля для формы секрета: символы или парольная фраза.
  // Пароль создаётся на клиенте и попадает в форму только по кнопке «Использовать».
  let { onuse, onclose }: { onuse: (password: string) => void; onclose: () => void } = $props()

  let mode: 'chars' | 'words' = $state('chars')
  let length = $state(20)
  let words = $state(6)
  let lower = $state(true)
  let upper = $state(true)
  let digits = $state(true)
  let symbols = $state(true)
  let noAmbiguous = $state(false)
  let result = $state<Generated | null>(null)
  let error = $state('')

  async function generate() {
    error = ''
    try {
      result =
        mode === 'words'
          ? await api.generatePassphrase(Number(words))
          : await api.generatePassword({ length: Number(length), lower, upper, digits, symbols, noAmbiguous })
    } catch (e) {
      result = null
      error = errorText(e)
    }
  }

  // Новый пароль при любом изменении параметров.
  $effect(() => {
    void [mode, length, words, lower, upper, digits, symbols, noAmbiguous]
    generate()
  })

  let strengthClass = $derived(
    !result ? '' : result.entropyBits >= 100 ? 'great' : result.entropyBits >= 75 ? 'good' : result.entropyBits >= 60 ? 'fair' : 'weak',
  )
</script>

<div class="generator stack">
  <div class="tabs">
    <button type="button" class:selected={mode === 'chars'} onclick={() => (mode = 'chars')}>Символы</button>
    <button type="button" class:selected={mode === 'words'} onclick={() => (mode = 'words')}>Парольная фраза</button>
  </div>

  <div class="preview">
    <code>{result?.password ?? ''}</code>
    <button type="button" class="link" title="Другой пароль" onclick={generate}>↻</button>
  </div>
  {#if result}
    <div class="strength {strengthClass}">
      <div class="bar" style="width: {Math.min(100, (result.entropyBits / 128) * 100)}%"></div>
    </div>
    <span class="muted small">Надёжность: {result.entropyBits} бит — {result.strength}</span>
  {/if}
  {#if error}<p class="error">{error}</p>{/if}

  {#if mode === 'chars'}
    <label>
      <span>Длина: {length}</span>
      <input type="range" min="8" max="64" bind:value={length} />
    </label>
    <div class="checks">
      <label class="check"><input type="checkbox" bind:checked={lower} /> a–z</label>
      <label class="check"><input type="checkbox" bind:checked={upper} /> A–Z</label>
      <label class="check"><input type="checkbox" bind:checked={digits} /> 0–9</label>
      <label class="check"><input type="checkbox" bind:checked={symbols} /> !@#…</label>
      <label class="check"><input type="checkbox" bind:checked={noAmbiguous} /> без 0/O, 1/l/I</label>
    </div>
  {:else}
    <label>
      <span>Слов: {words}</span>
      <input type="range" min="4" max="10" bind:value={words} />
    </label>
    <p class="muted small" style="margin: 0">
      Случайные слова из словаря EFF: легко запомнить и набрать. Для паролей сайтов удобнее символы — их всё равно копируют из хранилища.
    </p>
  {/if}

  <div class="row">
    <span class="spacer"></span>
    <button type="button" onclick={onclose}>Закрыть</button>
    <button type="button" class="primary" disabled={!result} onclick={() => result && onuse(result.password)}>Использовать</button>
  </div>
</div>

<style>
  .generator { padding: 14px; background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius); }
  .tabs { display: flex; gap: 6px; }
  .tabs button { padding: 4px 10px; font-size: 12px; }
  .tabs button.selected { border-color: var(--accent); color: var(--accent); }
  .preview { display: flex; align-items: center; gap: 6px; }
  .preview code { flex: 1; padding: 10px; background: var(--panel-2); border-radius: 6px; font-size: 15px; word-break: break-all; user-select: text; min-height: 1.2em; }
  .preview button { font-size: 18px; }
  .strength { height: 4px; background: var(--panel-2); border-radius: 2px; overflow: hidden; }
  .bar { height: 100%; transition: width 0.2s; }
  .weak .bar { background: var(--danger); }
  .fair .bar { background: #e0b44c; }
  .good .bar { background: var(--ok); }
  .great .bar { background: var(--accent); }
  .small { font-size: 12px; }
  .checks { display: flex; flex-wrap: wrap; gap: 12px; }
  .check { flex-direction: row; align-items: center; gap: 6px; color: var(--text); }
  .check input { width: auto; }
  input[type='range'] { padding: 0; }
</style>
