<script lang="ts">
  // Pestaña "Logs". Sintaxis clásica de Svelte (sin runes), ver CLAUDE.md.
  import { onMount, onDestroy, tick } from 'svelte'
  import { GetLogs, ReadLog, ClearLog, OpenLogsFolder } from '../wailsjs/go/main/App.js'
  import { ClipboardSetText } from '../wailsjs/runtime/runtime.js'
  import type { logview } from '../wailsjs/go/models'

  let logs: logview.Log[] = []
  let selected = 'kiltro'
  let result: logview.Result | null = null
  let error = ''
  let follow = true          // actualizar cada 2 segundos
  let confirmClear = false
  let copied = false
  let box: HTMLPreElement

  let timer: ReturnType<typeof setInterval> | null = null

  // Marca las líneas con errores/advertencias para pintarlas de otro color.
  function lineClass(line: string): string {
    const l = line.toLowerCase()
    if (l.includes('[error]') || l.includes('error:') || l.includes('fatal') || l.includes('[emerg]') || l.includes('[crit]')) return 'err'
    if (l.includes('advertencia') || l.includes('[warn]') || l.includes('[warning]') || l.includes('warning:')) return 'warn'
    return ''
  }

  function sizeLabel(bytes: number): string {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / 1024 / 1024).toFixed(1) + ' MB'
  }

  async function load() {
    // Si el usuario está mirando el final, después de actualizar se baja solo.
    const atBottom = !box || box.scrollHeight - box.scrollTop - box.clientHeight < 40
    try {
      result = await ReadLog(selected)
      error = ''
    } catch (e) {
      error = String(e)
    }
    await tick()
    if (atBottom && box) box.scrollTop = box.scrollHeight
  }

  async function changeLog() {
    confirmClear = false
    result = null
    await load()
    if (box) box.scrollTop = box.scrollHeight
  }

  async function copy() {
    if (!result) return
    await ClipboardSetText(result.lines.join('\n'))
    copied = true
    setTimeout(() => (copied = false), 1500)
  }

  async function clear() {
    try {
      await ClearLog(selected)
    } catch (e) {
      error = String(e)
    }
    confirmClear = false
    await load()
  }

  function startTimer() {
    stopTimer()
    timer = setInterval(load, 2000)
  }
  function stopTimer() {
    if (timer) clearInterval(timer)
    timer = null
  }
  $: follow ? startTimer() : stopTimer()

  $: current = logs.find((l) => l.id === selected)

  onMount(async () => {
    logs = await GetLogs()
    await changeLog()
  })
  // El componente se destruye al cambiar de pestaña: deja de leer el archivo.
  onDestroy(stopTimer)
</script>

<section>
  <div class="toolbar">
    <select bind:value={selected} on:change={changeLog}>
      {#each logs as l (l.id)}
        <option value={l.id}>{l.label}</option>
      {/each}
    </select>
    <label class="check"><input type="checkbox" bind:checked={follow} /> Actualizar solo</label>
    <button on:click={load}>Recargar</button>
    <button on:click={copy} disabled={!result || result.lines.length === 0}>{copied ? 'Copiado ✓' : 'Copiar'}</button>
    <button on:click={() => OpenLogsFolder()}>Abrir carpeta</button>
    {#if !confirmClear}
      <button on:click={() => (confirmClear = true)} disabled={!result || result.size === 0}>Vaciar</button>
    {:else}
      <span class="warn">¿Vaciar este log?</span>
      <button class="danger" on:click={clear}>Sí, vaciar</button>
      <button on:click={() => (confirmClear = false)}>Cancelar</button>
    {/if}
  </div>

  {#if current}
    <p class="muted small">
      <code>{current.path}</code>
      {#if result?.exists} — {sizeLabel(result.size)}{#if result.truncated}, se muestran las últimas {result.lines.length} líneas{/if}{/if}
    </p>
  {/if}

  {#if error}
    <p class="error">{error}</p>
  {/if}

  <pre bind:this={box}>{#if result && !result.exists}<span class="muted">Este log todavía no existe (se crea cuando el servicio arranca).</span>{:else if result && result.lines.length === 0}<span class="muted">(vacío)</span>{:else if result}{#each result.lines as line}<span class={lineClass(line)}>{line}</span>
{/each}{/if}</pre>
</section>

<style>
  section {
    display: flex;
    flex-direction: column;
    height: calc(100vh - 11rem);
  }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
  }
  select {
    padding: 0.25rem 0.4rem;
    border-radius: 3px;
  }
  .check {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    margin: 0 0.4rem;
  }
  pre {
    flex: 1;
    overflow: auto;
    margin: 0.4rem 0 0;
    padding: 0.6rem 0.8rem;
    background: #111a26;
    border: 1px solid #2c3e56;
    border-radius: 4px;
    font: 12px/1.45 Consolas, 'Cascadia Mono', monospace;
    text-align: left;
    white-space: pre-wrap;
    word-break: break-all;
  }
  .err {
    color: #ff8a80;
  }
  .warn {
    color: #ffd54f;
  }
  .muted {
    opacity: 0.6;
  }
  .small {
    font-size: 0.85em;
    margin: 0.5rem 0 0;
  }
  .error {
    color: #ff8a80;
  }
  button {
    padding: 0.3rem 0.8rem;
    border: none;
    border-radius: 3px;
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .danger {
    background: #c62828;
    color: white;
  }
</style>
