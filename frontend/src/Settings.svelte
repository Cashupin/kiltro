<script lang="ts">
  // Pantalla de configuración. Sintaxis clásica de Svelte (sin runes), ver CLAUDE.md.
  import { onMount } from 'svelte'
  import { GetSettings, SaveSettings, ChooseFolder, UninstallCA } from '../wailsjs/go/main/App.js'
  import { config } from '../wailsjs/go/models'

  // Valores del formulario
  let httpPort = 80
  let httpsPort = 443
  let mysqlPort = 3306
  let phpBasePort = 9000
  let phpWorkers = 4
  let lan = false
  let www = ''          // '' = carpeta por defecto
  let defaultWww = ''

  let loaded: config.Settings | null = null // lo último guardado (para avisos)
  let loadError = ''
  let saving = false
  let msg = ''
  let msgIsError = false

  let confirmUninstall = false
  let uninstalling = false
  let caMsg = ''
  let caMsgIsError = false

  $: phpLastPort = Number(phpBasePort) + Number(phpWorkers) - 1
  $: mysqlChanged = loaded !== null && Number(mysqlPort) !== loaded.mysqlPort

  async function load() {
    const v = await GetSettings()
    const s = v.settings
    loaded = s
    httpPort = s.httpPort
    httpsPort = s.httpsPort
    mysqlPort = s.mysqlPort
    phpBasePort = s.phpBasePort
    phpWorkers = s.phpWorkers
    lan = !!s.lan
    www = s.www ?? ''
    defaultWww = v.defaultWww
    loadError = v.error
  }

  async function chooseFolder() {
    const dir = await ChooseFolder(www || defaultWww)
    if (dir) www = dir
  }

  const serviceNames: Record<string, string> = { php: 'PHP', mysql: 'MySQL', nginx: 'Nginx', redis: 'Redis' }

  async function save() {
    saving = true
    msg = ''
    try {
      const restarted = await SaveSettings(config.Settings.createFrom({
        httpPort: Number(httpPort),
        httpsPort: Number(httpsPort),
        mysqlPort: Number(mysqlPort),
        phpBasePort: Number(phpBasePort),
        phpWorkers: Number(phpWorkers),
        lan,
        www: www.trim(),
      }))
      msgIsError = false
      msg = restarted && restarted.length > 0
        ? 'Guardado. Reiniciado: ' + restarted.map((n) => serviceNames[n] ?? n).join(', ') + '.'
        : 'Guardado. Se aplica la próxima vez que inicies los servicios.'
      await load()
    } catch (e) {
      msgIsError = true
      msg = String(e)
    } finally {
      saving = false
    }
  }

  async function uninstallCA() {
    uninstalling = true
    caMsg = ''
    try {
      await UninstallCA()
      caMsgIsError = false
      caMsg = 'Certificado de Kiltro quitado de Windows. HTTPS quedó desactivado.'
    } catch (e) {
      caMsgIsError = true
      caMsg = String(e)
    } finally {
      uninstalling = false
      confirmUninstall = false
    }
  }

  onMount(load)
</script>

<section>
  {#if loadError}
    <p class="error">{loadError}. Se muestran los valores por defecto; al guardar se corrige el archivo.</p>
  {/if}

  <h2>Puertos</h2>
  <div class="grid">
    <label for="http">Nginx HTTP</label>
    <input id="http" type="number" min="1" max="65535" bind:value={httpPort} />

    <label for="https">Nginx HTTPS</label>
    <input id="https" type="number" min="1" max="65535" bind:value={httpsPort} />

    <label for="mysql">MySQL</label>
    <input id="mysql" type="number" min="1" max="65535" bind:value={mysqlPort} />

    <label for="phpbase">PHP (primer puerto)</label>
    <div>
      <input id="phpbase" type="number" min="1" max="65535" bind:value={phpBasePort} />
      <span class="muted">workers en {phpBasePort}–{phpLastPort}</span>
    </div>

    <label for="workers">Workers de PHP</label>
    <div>
      <input id="workers" type="number" min="1" max="16" bind:value={phpWorkers} />
      <span class="muted">peticiones PHP que se atienden a la vez</span>
    </div>
  </div>
  {#if mysqlChanged}
    <p class="warn">Si cambias el puerto de MySQL, actualiza <code>DB_PORT</code> en el <code>.env</code> de tus proyectos.</p>
  {/if}

  <h2>Red</h2>
  <label class="check">
    <input type="checkbox" bind:checked={lan} />
    Accesible desde la red local
  </label>
  <p class="muted small">
    Apagado (recomendado): los sitios solo responden en esta PC. Encendido: otros equipos de tu red
    (ej. tu celular) pueden entrar por la IP de esta PC, y Windows puede preguntar por el firewall.
    Adminer sigue siendo solo para esta PC.
  </p>

  <h2>Carpeta de proyectos</h2>
  <div class="row">
    <input class="path" type="text" bind:value={www} placeholder={defaultWww} />
    <button on:click={chooseFolder}>Elegir…</button>
    {#if www}
      <button on:click={() => (www = '')} title={defaultWww}>Usar la predeterminada</button>
    {/if}
  </div>
  <p class="muted small">Cada carpeta dentro es un sitio <code>&lt;carpeta&gt;.test</code>.</p>

  <p class="row">
    <button class="primary" on:click={save} disabled={saving}>
      {saving ? 'Guardando…' : 'Guardar'}
    </button>
    {#if msg}<span class={msgIsError ? 'error' : 'ok'}>{msg}</span>{/if}
  </p>

  <h2>HTTPS</h2>
  <p class="muted small">
    Quita de Windows el certificado de Kiltro que se instaló al activar HTTPS. Los navegadores dejan
    de confiar en <code>https://*.test</code>. Windows pide confirmar.
  </p>
  <p class="row">
    {#if !confirmUninstall}
      <button on:click={() => (confirmUninstall = true)} disabled={uninstalling}>Desinstalar certificado de Kiltro</button>
    {:else}
      <span class="warn">¿Seguro?</span>
      <button class="danger" on:click={uninstallCA} disabled={uninstalling}>
        {uninstalling ? 'Esperando confirmación…' : 'Sí, desinstalar'}
      </button>
      <button on:click={() => (confirmUninstall = false)} disabled={uninstalling}>Cancelar</button>
    {/if}
  </p>
  {#if caMsg}<p class={caMsgIsError ? 'error' : 'ok'}>{caMsg}</p>{/if}
</section>

<style>
  section {
    max-width: 760px;
  }
  h2 {
    margin-top: 1.8rem;
    font-size: 1.1rem;
  }
  .grid {
    display: grid;
    grid-template-columns: 180px 1fr;
    gap: 0.5rem 1rem;
    align-items: center;
  }
  input[type='number'] {
    width: 7rem;
  }
  input[type='number'],
  input[type='text'] {
    padding: 0.3rem 0.5rem;
    border-radius: 3px;
    border: 1px solid #555;
  }
  .path {
    flex: 1;
    min-width: 0;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .check {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .muted {
    opacity: 0.6;
    margin-left: 0.5rem;
  }
  .small {
    font-size: 0.9em;
    margin-left: 0;
  }
  .warn {
    color: #ffd54f;
  }
  .ok {
    color: #81c784;
  }
  .error {
    color: #ff8a80;
    white-space: pre-wrap;
  }
  button {
    padding: 0.3rem 0.8rem;
    border: none;
    border-radius: 3px;
    cursor: pointer;
  }
  button:disabled {
    cursor: wait;
    opacity: 0.6;
  }
  .primary {
    background: #2e7d32;
    color: white;
    min-width: 100px;
  }
  .danger {
    background: #c62828;
    color: white;
  }
</style>
