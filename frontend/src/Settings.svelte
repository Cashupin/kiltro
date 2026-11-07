<script lang="ts">
  // Pantalla de configuración. Sintaxis clásica de Svelte (sin runes), ver CLAUDE.md.
  import { onMount } from 'svelte'
  import { GetSettings, SaveSettings, ChooseFolder, UninstallCA, GetAppRoot } from '../wailsjs/go/main/App.js'
  import { config } from '../wailsjs/go/models'

  // Valores del formulario
  let httpPort = 80
  let httpsPort = 443
  let mysqlPort = 3306
  let redisPort = 6379
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
    redisPort = s.redisPort
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
        redisPort: Number(redisPort),
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

  let root = '…' // carpeta de la app (sección "Acerca de")
  onMount(async () => {
    root = await GetAppRoot()
    await load()
  })
</script>

<section class="settings">
  {#if loadError}
    <p class="error">{loadError}. Se muestran los valores por defecto; al guardar se corrige el archivo.</p>
  {/if}

  <div class="card">
    <div class="card-title">Puertos</div>
    <div class="body grid">
      <label for="http">Nginx HTTP</label>
      <input id="http" type="number" min="1" max="65535" bind:value={httpPort} />

      <label for="https">Nginx HTTPS</label>
      <input id="https" type="number" min="1" max="65535" bind:value={httpsPort} />

      <label for="mysql">MySQL</label>
      <input id="mysql" type="number" min="1" max="65535" bind:value={mysqlPort} />

      <label for="redis">Redis</label>
      <input id="redis" type="number" min="1" max="65535" bind:value={redisPort} />

      <label for="phpbase">PHP (primer puerto)</label>
      <div>
        <input id="phpbase" type="number" min="1" max="65535" bind:value={phpBasePort} />
        <span class="muted hint">workers en {phpBasePort}–{phpLastPort}</span>
      </div>

      <label for="workers">Workers de PHP</label>
      <div>
        <input id="workers" type="number" min="1" max="16" bind:value={phpWorkers} />
        <span class="muted hint">peticiones PHP que se atienden a la vez</span>
      </div>
    </div>
    {#if mysqlChanged}
      <p class="body warn">Si cambias el puerto de MySQL, actualiza <code>DB_PORT</code> en el <code>.env</code> de tus proyectos.</p>
    {/if}
  </div>

  <div class="card">
    <div class="card-title">Red</div>
    <div class="body">
      <label class="check">
        <input type="checkbox" bind:checked={lan} />
        Accesible desde la red local
      </label>
      <p class="muted small">
        Apagado (recomendado): los sitios solo responden en esta PC. Encendido: otros equipos de tu red
        (ej. tu celular) pueden entrar por la IP de esta PC, y Windows puede preguntar por el firewall.
        Adminer sigue siendo solo para esta PC.
      </p>
    </div>
  </div>

  <div class="card">
    <div class="card-title">Carpeta de proyectos</div>
    <div class="body">
      <div class="row">
        <input class="path" type="text" bind:value={www} placeholder={defaultWww} />
        <button on:click={chooseFolder}>Elegir…</button>
        {#if www}
          <button class="ghost" on:click={() => (www = '')} title={defaultWww}>Usar la predeterminada</button>
        {/if}
      </div>
      <p class="muted small">Cada carpeta dentro es un sitio <code>&lt;carpeta&gt;.test</code>.</p>
    </div>
  </div>

  <div class="save row">
    <button class="primary" on:click={save} disabled={saving}>
      {saving ? 'Guardando…' : 'Guardar cambios'}
    </button>
    {#if msg}<span class={msgIsError ? 'error' : 'ok'}>{msg}</span>{/if}
  </div>

  <div class="card">
    <div class="card-title">HTTPS</div>
    <div class="body">
      <p class="muted small first">
        Quita de Windows el certificado de Kiltro que se instaló al activar HTTPS. Los navegadores dejan
        de confiar en <code>https://*.test</code>. Windows pide confirmar.
      </p>
      <div class="row">
        {#if !confirmUninstall}
          <button on:click={() => (confirmUninstall = true)} disabled={uninstalling}>Desinstalar certificado de Kiltro</button>
        {:else}
          <span class="warn">¿Seguro?</span>
          <button class="danger" on:click={uninstallCA} disabled={uninstalling}>
            {uninstalling ? 'Esperando confirmación…' : 'Sí, desinstalar'}
          </button>
          <button class="ghost" on:click={() => (confirmUninstall = false)} disabled={uninstalling}>Cancelar</button>
        {/if}
      </div>
      {#if caMsg}<p class={caMsgIsError ? 'error' : 'ok'}>{caMsg}</p>{/if}
    </div>
  </div>

  <div class="card">
    <div class="card-title">Acerca de</div>
    <div class="body">
      <p class="small first">Carpeta de la app: <code class="sel">{root}</code></p>
      <p class="muted small">Ahí están <code>bin\</code>, <code>etc\</code>, <code>data\</code>, <code>logs\</code> y <code>config.json</code>.</p>
    </div>
  </div>
</section>

<style>
  .settings {
    max-width: 780px;
  }
  .body {
    padding: 0.2rem 1rem 0.8rem;
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
  .hint {
    margin-left: 0.5rem;
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
  .save {
    margin: 0 0 1rem;
  }
  .check {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .small {
    font-size: 0.92em;
    margin: 0.5rem 0 0;
  }
  .first {
    margin-top: 0;
    margin-bottom: 0.6rem;
  }
  .sel {
    user-select: all;
  }
</style>
