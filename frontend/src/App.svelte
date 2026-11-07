<script lang="ts">
  // Sintaxis clásica de Svelte (sin runes), ver CLAUDE.md.
  import { onMount, onDestroy } from 'svelte'
  import {
    GetServices, StartService, StopService, StartAll, StopAll, GetSites, ReloadSites,
    GetHostsStatus, SyncHosts, OpenAdminer, GetPHPVersions, SetPHPVersion,
    OpenSiteFolder, OpenSiteTerminal, CreateSite,
    GetSSLStatus, EnableSSL, DisableSSL,
  } from '../wailsjs/go/main/App.js'
  import { EventsOn, BrowserOpenURL } from '../wailsjs/runtime/runtime.js'
  import Settings from './Settings.svelte'
  import Logs from './Logs.svelte'
  import logo from './assets/images/kiltro.svg' // mismo dibujo que el ícono del exe (build/kiltro.svg)
  import type { services, vhosts } from '../wailsjs/go/models'

  let list: services.Info[] = []
  let errorMsg: string = ''         // error general (ej. puerto ocupado)
  let busy: Record<string, boolean> = {} // servicio con una acción en curso

  let sites: vhosts.Site[] = []
  let skipped: vhosts.Skipped[] = []
  let reloading = false
  let sitesMsg = '' // resultado de "Recargar sitios"

  let hostsStatus: services.HostsStatus | null = null
  let syncing = false

  let phpVersions: services.PHPVersion[] = []
  let phpChoice = ''         // nombre de la carpeta elegida en el desplegable
  let switchingPhp = false   // true mientras se guarda/reinicia PHP

  // "8.4.27 (NTS)"; si la carpeta no trae versión en el nombre, se muestra el nombre.
  function phpLabel(v: services.PHPVersion): string {
    return (v.version || v.name) + (v.nts ? ' (NTS)' : ' (TS)')
  }

  async function refreshPhpVersions() {
    try {
      phpVersions = (await GetPHPVersions()) ?? []
      phpChoice = phpVersions.find((v) => v.selected)?.name ?? ''
    } catch (e) {
      errorMsg = String(e)
    }
  }

  async function changePhp() {
    errorMsg = ''
    switchingPhp = true
    try {
      await SetPHPVersion(phpChoice)
    } catch (e) {
      errorMsg = String(e)
    } finally {
      switchingPhp = false
      await refreshPhpVersions()
      await refresh()
    }
  }

  // Adminer está instalado si aparece entre los sitios (como herramienta).
  $: hasAdminer = sites.some((x) => x.tool && x.name === 'adminer')
  // En la lista solo se muestran los proyectos de www; las herramientas (Adminer)
  // se usan desde su botón.
  $: projects = sites.filter((x) => !x.tool)

  // "Gestor BD": inicia lo que falte (PHP, MySQL, Nginx) y abre Adminer.
  let openingAdminer = false
  async function openAdminer() {
    errorMsg = ''
    openingAdminer = true
    try {
      await OpenAdminer()
    } catch (e) {
      errorMsg = String(e)
    } finally {
      openingAdminer = false
      await refresh()
    }
  }

  // Errores de los botones de cada sitio (Carpeta, Terminal).
  async function siteAction(f: () => Promise<void>) {
    sitesMsg = ''
    try {
      await f()
    } catch (e) {
      sitesMsg = String(e)
    }
  }

  // "Nuevo sitio"
  let creating = false     // se muestra el campo para escribir el nombre
  let newName = ''
  let createMsg = ''
  let createIsError = false
  async function createSite() {
    createMsg = ''
    try {
      const domain = await CreateSite(newName)
      createIsError = false
      createMsg = 'Creado ' + domain + ': pon tu proyecto en su carpeta.'
      newName = ''
      creating = false
    } catch (e) {
      createIsError = true
      createMsg = String(e)
    } finally {
      await refreshSites()
      await refreshHosts()
    }
  }

  const labels: Record<string, string> = {
    stopped: 'Detenido',
    starting: 'Iniciando…',
    running: 'Corriendo',
    stopping: 'Deteniendo…',
    error: 'Error',
  }

  const names: Record<string, string> = {
    php: 'PHP',
    nginx: 'Nginx',
    mysql: 'MySQL',
    redis: 'Redis',
  }

  // [9000, 9001, 9002, 9003] → ":9000-9003"; [80] → ":80"; [80, 443] → ":80, 443"
  function portsLabel(ports: number[]): string {
    if (!ports || ports.length === 0) return ''
    const consecutive = ports.every((p, i) => i === 0 || p === ports[i - 1] + 1)
    if (consecutive && ports.length > 2) return ':' + ports[0] + '-' + ports[ports.length - 1]
    return ':' + ports.join(', ')
  }

  // --- HTTPS ---
  let ssl: services.SSLStatus | null = null
  let sslBusy = false
  let sslMsg = '' // error al activar/desactivar

  $: scheme = ssl?.enabled ? 'https://' : 'http://'

  async function refreshSsl() {
    try {
      ssl = await GetSSLStatus()
    } catch (e) {
      ssl = null
    }
  }

  async function toggleSsl() {
    sslBusy = true
    sslMsg = ''
    try {
      if (ssl?.enabled) {
        await DisableSSL()
      } else {
        await EnableSSL()
      }
    } catch (e) {
      sslMsg = String(e)
    } finally {
      sslBusy = false
      await refreshSsl()
      await refresh()
    }
  }

  async function refresh() {
    try {
      list = await GetServices()
    } catch (e) {
      errorMsg = String(e)
    }
  }

  async function refreshSites() {
    try {
      const r = await GetSites()
      sites = r.sites ?? []
      skipped = r.skipped ?? []
    } catch (e) {
      sitesMsg = String(e)
    }
  }

  async function refreshHosts() {
    try {
      hostsStatus = await GetHostsStatus()
    } catch (e) {
      hostsStatus = null
    }
  }

  async function syncHosts() {
    syncing = true
    try {
      await SyncHosts()
    } catch (e) {
      // el error queda en hostsStatus.error
    } finally {
      syncing = false
      await refreshHosts()
    }
  }

  function openSite(domain: string) {
    BrowserOpenURL(scheme + domain)
  }

  async function reloadSites() {
    reloading = true
    sitesMsg = ''
    try {
      await ReloadSites()
      sitesMsg = 'Sitios recargados'
    } catch (e) {
      sitesMsg = String(e)
    } finally {
      reloading = false
      await refreshSites()
      await refreshHosts()
    }
  }

  // "Iniciar todo" / "Detener todo" (barra superior)
  let allBusy = false
  async function startAll() {
    errorMsg = ''
    allBusy = true
    try {
      await StartAll()
    } catch (e) {
      errorMsg = String(e)
    } finally {
      allBusy = false
      await refresh()
    }
  }
  async function stopAll() {
    errorMsg = ''
    allBusy = true
    try {
      await StopAll()
    } catch (e) {
      errorMsg = String(e)
    } finally {
      allBusy = false
      await refresh()
    }
  }
  $: runningCount = list.filter((s) => s.status === 'running').length
  const isOn = (s: services.Info) => s.status === 'running' || s.status === 'starting'

  // Vista previa del dominio del sitio nuevo: "Mi Tienda" -> "mi-tienda.test"
  $: newSiteDomain = newName.trim() ? newName.trim().toLowerCase().replace(/[ _]/g, '-') + '.test' : ''

  async function toggle(s: services.Info) {
    errorMsg = ''
    busy = { ...busy, [s.name]: true }
    try {
      if (s.status === 'running' || s.status === 'starting') {
        await StopService(s.name)
      } else {
        await StartService(s.name)
      }
    } catch (e) {
      errorMsg = String(e)
    } finally {
      busy = { ...busy, [s.name]: false }
      await refresh()
    }
  }

  // El backend emite "services:changed" cada vez que cambia un estado
  // (incluido si un servicio se cae solo).
  // "hosts:changed" llega cuando termina una actualización del hosts en segundo plano.
  let offs: Array<() => void> = []

  // Pestaña visible. Al volver a "Servicios" se refresca todo (la configuración
  // pudo cambiar puertos, la carpeta de proyectos o HTTPS).
  let tab: 'services' | 'logs' | 'settings' = 'services'
  async function showTab(t: 'services' | 'logs' | 'settings') {
    tab = t
    if (t === 'services') {
      await refresh()
      await refreshSites()
      await refreshHosts()
      await refreshSsl()
    }
  }

  onMount(async () => {
    offs.push(EventsOn('services:changed', refresh))
    offs.push(EventsOn('hosts:changed', refreshHosts))
    // Errores de acciones hechas desde el menú de la bandeja.
    offs.push(EventsOn('app:error', (msg: string) => { errorMsg = msg }))
    await refresh()
    await refreshPhpVersions()
    await refreshSites()
    await refreshHosts()
    await refreshSsl()
  })

  onDestroy(() => offs.forEach((off) => off()))
</script>

<div class="layout">
  <!-- Barra superior: nombre, resumen y acciones globales -->
  <header class="topbar">
    <div class="brand">
      <img class="logo" src={logo} alt="" />
      <span class="title">Kiltro</span>
      {#if list.length > 0}
        <span class="summary">
          <span class="dot {runningCount === list.length ? 'running' : runningCount > 0 ? 'warn' : ''}"></span>
          {runningCount} de {list.length} servicios corriendo
        </span>
      {/if}
    </div>
    <div class="top-actions">
      <button class="primary" on:click={startAll} disabled={allBusy || runningCount === list.length}>▶ Iniciar todo</button>
      <button on:click={stopAll} disabled={allBusy || runningCount === 0}>■ Detener todo</button>
    </div>
  </header>

  <nav class="tabs">
    <button class:active={tab === 'services'} on:click={() => showTab('services')}>Servicios</button>
    <button class:active={tab === 'logs'} on:click={() => showTab('logs')}>Logs</button>
    <button class:active={tab === 'settings'} on:click={() => showTab('settings')}>Configuración</button>
  </nav>

  <main>
    {#if tab === 'settings'}
      <Settings />
    {:else if tab === 'logs'}
      <Logs />
    {:else}

    {#if errorMsg}
      <div class="alert">{errorMsg}<button class="ghost small" on:click={() => (errorMsg = '')}>✕</button></div>
    {/if}

    <!-- Servicios -->
    <section class="card">
      <div class="card-title">Servicios</div>
      {#each list as s (s.name)}
        <div class="svc">
          <span class="dot {s.status}" title={labels[s.status] ?? s.status}></span>
          <div class="svc-name">
            <span class="name">{names[s.name] ?? s.name}</span>
            {#if s.name === 'php' && phpVersions.length > 0}
              <!-- Cambiar la versión reinicia PHP si está corriendo -->
              <select bind:value={phpChoice} on:change={changePhp}
                disabled={switchingPhp || busy['php'] || phpVersions.length < 2}
                title="Versión de PHP (si PHP está corriendo, se reinicia)">
                {#each phpVersions as v (v.name)}
                  <option value={v.name}>{phpLabel(v)}</option>
                {/each}
              </select>
            {/if}
          </div>
          <code class="ports">{portsLabel(s.ports)}</code>
          <span class="state {s.status}">
            {labels[s.status] ?? s.status}{#if s.detail}<span class="muted"> · {s.detail}</span>{/if}
          </span>
          <div class="svc-actions">
            {#if s.name === 'mysql' && hasAdminer}
              <button class="ghost" on:click={openAdminer} disabled={openingAdminer}
                title="Abre Adminer en el navegador (inicia PHP, MySQL y Nginx si hace falta)">
                {openingAdminer ? 'Iniciando…' : 'Gestor BD'}
              </button>
            {/if}
            <button class={isOn(s) ? '' : 'outline'} on:click={() => toggle(s)} disabled={busy[s.name] || allBusy}>
              {isOn(s) ? '■ Detener' : '▶ Iniciar'}
            </button>
          </div>
        </div>
        {#if s.error}
          <div class="svc-error error">{s.error}</div>
        {/if}
      {/each}
    </section>

    <!-- Sitios -->
    <section class="card">
      <div class="card-title">
        Sitios <span class="count">{projects.length}</span>
        <span class="spacer"></span>
        <button class="ghost small" on:click={reloadSites} disabled={reloading} title="Volver a leer la carpeta de proyectos">
          {reloading ? 'Recargando…' : '↻ Recargar'}
        </button>
        <button class="small primary" on:click={() => { creating = !creating; createMsg = '' }}>＋ Nuevo sitio</button>
      </div>

      {#if creating}
        <form class="newsite" on:submit|preventDefault={createSite}>
          <input bind:value={newName} placeholder="nombre (ej. mi-tienda)" />
          <span class="muted">{newSiteDomain}</span>
          <span class="spacer"></span>
          <button type="submit" class="primary small" disabled={!newName.trim()}>Crear</button>
          <button type="button" class="ghost small" on:click={() => (creating = false)}>Cancelar</button>
        </form>
      {/if}
      {#if createMsg}
        <p class="msg {createIsError ? 'error' : 'ok'}">{createMsg}</p>
      {/if}

      {#if projects.length === 0}
        <p class="msg muted">No hay proyectos. Pulsa "Nuevo sitio" o crea una carpeta dentro de la carpeta de proyectos y pulsa "Recargar".</p>
      {:else}
        {#each projects as site (site.name)}
          <div class="site">
            <div class="site-main">
              <a href={scheme + site.domain} on:click|preventDefault={() => openSite(site.domain)}>{scheme}{site.domain}</a>
              {#if site.laravel}<span class="badge">Laravel</span>{/if}
              <code class="path" title={site.root}>{site.root}</code>
            </div>
            <div class="site-actions">
              <button class="ghost small" on:click={() => openSite(site.domain)}>Abrir</button>
              <button class="ghost small" on:click={() => siteAction(() => OpenSiteFolder(site.name))}>Carpeta</button>
              <button class="ghost small" on:click={() => siteAction(() => OpenSiteTerminal(site.name))}>Terminal</button>
            </div>
          </div>
        {/each}
      {/if}

      {#each skipped as sk}
        <p class="msg warn">Ignorada <code>{sk.folder}</code>: {sk.reason}</p>
      {/each}
      {#if sitesMsg}
        <p class="msg {sitesMsg === 'Sitios recargados' ? 'muted' : 'error'}">{sitesMsg}</p>
      {/if}
    </section>

    <!-- Estado: archivo hosts y HTTPS -->
    <section class="status-row">
      {#if hostsStatus}
        <div class="card status">
          <div class="status-head">
            <span class="dot {hostsStatus.upToDate ? 'ok' : 'warn'}"></span>
            <span class="label">Archivo hosts</span>
            <span class={hostsStatus.upToDate ? 'ok' : 'warn'}>{hostsStatus.upToDate ? 'al día' : 'desactualizado'}</span>
            <span class="spacer"></span>
            {#if !hostsStatus.upToDate}
              <button class="small" on:click={syncHosts} disabled={syncing}>
                {syncing ? 'Esperando permiso…' : 'Actualizar'}
              </button>
            {/if}
          </div>
          {#if hostsStatus.error}<p class="msg error">{hostsStatus.error}</p>{/if}
        </div>
      {/if}

      {#if ssl}
        <div class="card status">
          <div class="status-head">
            <span class="dot {ssl.enabled ? 'ok' : ''}"></span>
            <span class="label">HTTPS</span>
            <span class={ssl.enabled ? 'ok' : 'muted'}>{ssl.enabled ? 'activado' : 'desactivado'}</span>
            <span class="spacer"></span>
            {#if ssl.available}
              <button class="small" on:click={toggleSsl} disabled={sslBusy}
                title={ssl.enabled ? 'Volver a solo http://' : 'Windows pedirá confirmar que confías en el certificado de Kiltro'}>
                {#if sslBusy}
                  {ssl.enabled ? 'Desactivando…' : 'Esperando confirmación…'}
                {:else}
                  {ssl.enabled ? 'Desactivar' : 'Activar'}
                {/if}
              </button>
            {/if}
          </div>
          {#if ssl.error}<p class="msg muted">{ssl.error}</p>{/if}
          {#if sslMsg}<p class="msg error">{sslMsg}</p>{/if}
        </div>
      {/if}
    </section>
    {/if}
  </main>
</div>

<style>
  .layout {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }

  /* --- Barra superior --- */
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.8rem 1.5rem;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .logo {
    width: 30px;
    height: 30px;
  }
  .title {
    font-size: 1.15rem;
    font-weight: 600;
  }
  .summary {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-left: 0.8rem;
    color: var(--muted);
    font-size: 0.92em;
  }
  .top-actions {
    display: flex;
    gap: 0.5rem;
  }

  /* --- Pestañas --- */
  .tabs {
    display: flex;
    gap: 0.2rem;
    padding: 0 1.5rem;
    border-bottom: 1px solid var(--border);
  }
  .tabs button {
    background: transparent;
    border: none;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    padding: 0.6rem 0.9rem;
    color: var(--muted);
  }
  .tabs button:hover:not(:disabled) {
    background: transparent;
    color: var(--text);
  }
  .tabs button.active {
    color: var(--text);
    border-bottom-color: var(--accent);
    font-weight: 600;
  }

  main {
    flex: 1;
    overflow: auto;
    padding: 1.2rem 1.5rem;
  }

  .alert {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.6rem 0.9rem;
    margin-bottom: 1rem;
    border: 1px solid #7f1d1d;
    background: #3b1d1d;
    color: #fecaca;
    border-radius: var(--radius);
    white-space: pre-wrap;
  }

  /* --- Servicios --- */
  .svc {
    display: grid;
    /* Columnas fijas: así todas las filas quedan alineadas aunque una tenga más botones (MySQL). */
    grid-template-columns: 14px minmax(190px, 1.3fr) 110px 1fr 230px;
    align-items: center;
    gap: 0.8rem;
    padding: 0.55rem 1rem;
    border-top: 1px solid var(--border);
  }
  .svc:hover {
    background: rgba(255, 255, 255, 0.02);
  }
  .svc-name {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .name {
    font-weight: 600;
  }
  .svc-name select {
    padding: 0.15rem 0.35rem;
    font-size: 0.9em;
  }
  .ports {
    color: var(--muted);
  }
  .state.running {
    color: var(--ok);
  }
  .state.starting,
  .state.stopping {
    color: var(--warn);
  }
  .state.error {
    color: #fca5a5;
  }
  .state.stopped {
    color: var(--muted);
  }
  .svc-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.4rem;
  }
  .svc-actions button {
    min-width: 96px;
  }
  .svc-error {
    padding: 0 1rem 0.6rem 2.9rem;
    font-size: 0.92em;
  }

  /* --- Sitios --- */
  .count {
    font-weight: normal;
    color: var(--muted);
    background: var(--surface-2);
    border-radius: 999px;
    padding: 0 0.5rem;
    font-size: 0.85em;
  }
  .newsite {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 1rem 0.7rem;
  }
  .newsite input {
    width: 16rem;
  }
  .newsite .spacer {
    flex: 1;
  }
  .site {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.5rem 1rem;
    border-top: 1px solid var(--border);
  }
  .site:hover {
    background: rgba(255, 255, 255, 0.02);
  }
  .site-main {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
  }
  .site-main a {
    font-weight: 600;
  }
  .path {
    color: var(--muted);
    font-size: 0.85em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .site-actions {
    display: flex;
    gap: 0.2rem;
    flex: none;
  }
  .badge {
    padding: 0.05rem 0.45rem;
    border-radius: 4px;
    background: rgba(244, 81, 30, 0.18);
    color: #fb923c;
    font-size: 0.8em;
    font-weight: 600;
  }
  .msg {
    margin: 0.3rem 1rem 0.6rem;
  }

  /* --- Estado (hosts / HTTPS) --- */
  .status-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
  }
  .status {
    padding: 0.6rem 1rem;
    margin-bottom: 0;
  }
  .status-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .status-head .label {
    font-weight: 600;
  }
  .status-head .spacer {
    flex: 1;
  }
  .status .msg {
    margin: 0.4rem 0 0;
    font-size: 0.92em;
  }
</style>
