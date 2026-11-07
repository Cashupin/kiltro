<script lang="ts">
  // Pantalla de configuración. Sintaxis clásica de Svelte (sin runes), ver CLAUDE.md.
  import { onMount } from 'svelte'
  import {
    GetSettings, SaveSettings, ChooseFolder, UninstallCA, GetAppRoot,
    GetShimsStatus, AddShimsToPath, RemoveShimsFromPath, GetVersion,
  } from '../wailsjs/go/main/App.js'
  import { config, services } from '../wailsjs/go/models'
  import { BrowserOpenURL } from '../wailsjs/runtime/runtime.js'
  import { t, th, tr, langPref, supported, changeLanguage } from './i18n'

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
        ? tr('settings.savedRestarted', { services: restarted.map((n) => serviceNames[n] ?? n).join(', ') })
        : tr('settings.savedLater')
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
      caMsg = tr('ca.done')
    } catch (e) {
      caMsgIsError = true
      caMsg = String(e)
    } finally {
      uninstalling = false
      confirmUninstall = false
    }
  }

  // php y composer en la terminal (bin\shims en el PATH del usuario)
  let shims: services.ShimsStatus | null = null
  let shimsBusy = false
  let shimsMsg = ''
  let shimsMsgIsError = false

  async function loadShims() {
    try {
      shims = await GetShimsStatus()
    } catch (e) {
      shimsMsgIsError = true
      shimsMsg = String(e)
    }
  }

  async function toggleShims(add: boolean) {
    shimsBusy = true
    shimsMsg = ''
    try {
      if (add) {
        await AddShimsToPath()
        shimsMsg = tr('shims.added')
      } else {
        await RemoveShimsFromPath()
        shimsMsg = tr('shims.removed')
      }
      shimsMsgIsError = false
    } catch (e) {
      shimsMsgIsError = true
      shimsMsg = String(e)
    } finally {
      shimsBusy = false
      await loadShims()
    }
  }

  // Idioma: se aplica en el momento (no espera a "Guardar cambios").
  // Los nombres de los idiomas van siempre en su propio idioma.
  const langNames: Record<string, string> = { es: 'Español', en: 'English' }
  let langMsg = ''
  async function pickLanguage(e: Event) {
    langMsg = ''
    try {
      await changeLanguage((e.target as HTMLSelectElement).value)
    } catch (err) {
      langMsg = String(err)
    }
  }

  let root = '…' // carpeta de la app (sección "Acerca de")
  let version = ""
  onMount(async () => {
    root = await GetAppRoot()
    version = await GetVersion()
    await load()
    await loadShims()
  })
</script>

<section class="settings">
  {#if loadError}
    <p class="error">{$t('settings.loadError', { error: loadError })}</p>
  {/if}

  <div class="card">
    <div class="card-title">{$t('settings.ports')}</div>
    <div class="body grid">
      <label for="http">Nginx HTTP</label>
      <input id="http" type="number" min="1" max="65535" bind:value={httpPort} />

      <label for="https">Nginx HTTPS</label>
      <input id="https" type="number" min="1" max="65535" bind:value={httpsPort} />

      <label for="mysql">MySQL</label>
      <input id="mysql" type="number" min="1" max="65535" bind:value={mysqlPort} />

      <label for="redis">Redis</label>
      <input id="redis" type="number" min="1" max="65535" bind:value={redisPort} />

      <label for="phpbase">{$t('settings.phpFirstPort')}</label>
      <div>
        <input id="phpbase" type="number" min="1" max="65535" bind:value={phpBasePort} />
        <span class="muted hint">{$t('settings.workersRange', { from: phpBasePort, to: phpLastPort })}</span>
      </div>

      <label for="workers">{$t('settings.phpWorkers')}</label>
      <div>
        <input id="workers" type="number" min="1" max="16" bind:value={phpWorkers} />
        <span class="muted hint">{$t('settings.workersHint')}</span>
      </div>
    </div>
    {#if mysqlChanged}
      <p class="body warn">{@html $th('settings.mysqlPortWarn')}</p>
    {/if}
  </div>

  <div class="card">
    <div class="card-title">{$t('settings.network')}</div>
    <div class="body">
      <label class="check">
        <input type="checkbox" bind:checked={lan} />
        {$t('settings.lan')}
      </label>
      <p class="muted small">{$t('settings.lanHelp')}</p>
    </div>
  </div>

  <div class="card">
    <div class="card-title">{$t('settings.www')}</div>
    <div class="body">
      <div class="row">
        <input class="path" type="text" bind:value={www} placeholder={defaultWww} />
        <button on:click={chooseFolder}>{$t('settings.choose')}</button>
        {#if www}
          <button class="ghost" on:click={() => (www = '')} title={defaultWww}>{$t('settings.useDefault')}</button>
        {/if}
      </div>
      <p class="muted small">{@html $th('settings.wwwHelp')}</p>
    </div>
  </div>

  <div class="save row">
    <button class="primary" on:click={save} disabled={saving}>
      {saving ? $t('settings.saving') : $t('settings.save')}
    </button>
    {#if msg}<span class={msgIsError ? 'error' : 'ok'}>{msg}</span>{/if}
  </div>

  <div class="card">
    <div class="card-title">{$t('lang.title')}</div>
    <div class="body">
      <select value={$langPref} on:change={pickLanguage}>
        <option value="">{$t('lang.auto')}</option>
        {#each $supported as code}
          <option value={code}>{langNames[code] ?? code}</option>
        {/each}
      </select>
      {#if langMsg}<p class="error small">{langMsg}</p>{/if}
    </div>
  </div>

  <div class="card">
    <div class="card-title">
      <span class="dot" class:ok={shims?.inPath}></span>
      {$t('shims.title')}
    </div>
    <div class="body">
      <p class="muted small first">{@html $th('shims.help', { dir: shims?.dir ?? 'bin\\shims' })}</p>
      {#if shims}
        <p class="small first">
          {#if shims.php}PHP: <code>{shims.php}</code>{:else}<span class="warn">{$t('shims.noPhp')}</span>{/if}
          {#if !shims.composer}
            · <span class="warn">{@html $th('shims.noComposer')}</span>
          {/if}
        </p>
        <div class="row">
          {#if shims.inPath}
            <span class="ok">{$t('shims.inPath')}</span>
            <button on:click={() => toggleShims(false)} disabled={shimsBusy}>{$t('shims.remove')}</button>
          {:else}
            <button class="outline" on:click={() => toggleShims(true)} disabled={shimsBusy || !shims.php}>
              {$t('shims.add')}
            </button>
          {/if}
        </div>
        {#if shims.error}<p class="error small">{shims.error}</p>{/if}
      {/if}
      {#if shimsMsg}<p class="small {shimsMsgIsError ? 'error' : 'warn'}">{shimsMsg}</p>{/if}
    </div>
  </div>

  <div class="card">
    <div class="card-title">HTTPS</div>
    <div class="body">
      <p class="muted small first">{@html $th('ca.help')}</p>
      <div class="row">
        {#if !confirmUninstall}
          <button on:click={() => (confirmUninstall = true)} disabled={uninstalling}>{$t('ca.uninstall')}</button>
        {:else}
          <span class="warn">{$t('ca.sure')}</span>
          <button class="danger" on:click={uninstallCA} disabled={uninstalling}>
            {uninstalling ? $t('common.waitingConfirm') : $t('ca.yes')}
          </button>
          <button class="ghost" on:click={() => (confirmUninstall = false)} disabled={uninstalling}>{$t('common.cancel')}</button>
        {/if}
      </div>
      {#if caMsg}<p class={caMsgIsError ? 'error' : 'ok'}>{caMsg}</p>{/if}
    </div>
  </div>

  <div class="card">
    <div class="card-title">{$t('about.title')}</div>
    <div class="body">
      <p class="small first">Kiltro <strong>{version}</strong> · {$t('about.free')} · <a href="https://github.com/Cashupin/kiltro" on:click|preventDefault={() => BrowserOpenURL('https://github.com/Cashupin/kiltro')}>github.com/Cashupin/kiltro</a></p>
      <p class="small">{$t('about.root')} <code class="sel">{root}</code></p>
      <p class="muted small">{@html $th('about.rootHelp')}</p>
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
