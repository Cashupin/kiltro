<script lang="ts">
  // Sintaxis clásica de Svelte (sin runes), ver CLAUDE.md.
  import { onMount, onDestroy } from 'svelte'
  import {
    GetAppRoot, GetServices, StartService, StopService, GetSites, ReloadSites,
    GetHostsStatus, SyncHosts, OpenAdminer, GetPHPVersions, SetPHPVersion,
    GetSSLStatus, EnableSSL, DisableSSL,
  } from '../wailsjs/go/main/App.js'
  import { EventsOn, BrowserOpenURL } from '../wailsjs/runtime/runtime.js'
  import type { services, vhosts } from '../wailsjs/go/models'

  let root: string = '…'
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

  onMount(async () => {
    root = await GetAppRoot()
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

<main>
  <p class="root">Kiltro — raíz: <code>{root}</code></p>

  <table>
    <tbody>
      {#each list as s (s.name)}
        <tr>
          <td class="name">
            {names[s.name] ?? s.name}
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
          </td>
          <td class="ports">{portsLabel(s.ports)}</td>
          <td>
            <span class="status {s.status}">{labels[s.status] ?? s.status}</span>
            {#if s.detail}<span class="detail">{s.detail}</span>{/if}
          </td>
          <td>
            <button on:click={() => toggle(s)} disabled={busy[s.name]}>
              {s.status === 'running' || s.status === 'starting' ? 'Detener' : 'Iniciar'}
            </button>
            {#if s.name === 'mysql' && hasAdminer}
              <button on:click={() => OpenAdminer()} disabled={s.status !== 'running'}
                title="Abre Adminer en el navegador (necesita PHP, MySQL y Nginx corriendo)">Gestor BD</button>
            {/if}
          </td>
        </tr>
        {#if s.error}
          <tr><td colspan="4" class="error">{s.error}</td></tr>
        {/if}
      {/each}
    </tbody>
  </table>

  {#if errorMsg}
    <p class="error">{errorMsg}</p>
  {/if}

  <h2>
    Sitios
    <button on:click={reloadSites} disabled={reloading}>
      {reloading ? 'Recargando…' : 'Recargar sitios'}
    </button>
  </h2>

  {#if sites.length === 0}
    <p class="muted">No hay proyectos. Crea una carpeta dentro de <code>www\</code> y pulsa "Recargar sitios".</p>
  {:else}
    <table>
      <tbody>
        {#each sites as site (site.name)}
          <tr>
            <td class="name">
              <a href={scheme + site.domain} on:click|preventDefault={() => openSite(site.domain)}>{site.domain}</a>
            </td>
            <td>
              {#if site.laravel}<span class="badge">Laravel</span>{/if}
              {#if site.tool}<span class="badge tool">Herramienta</span>{/if}
            </td>
            <td class="muted"><code>{site.root}</code></td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}

  {#each skipped as sk}
    <p class="warn">Ignorada <code>www\{sk.folder}</code>: {sk.reason}</p>
  {/each}

  {#if sitesMsg}
    <p class={sitesMsg === 'Sitios recargados' ? 'muted' : 'error'}>{sitesMsg}</p>
  {/if}

  {#if hostsStatus}
    <p class="hosts">
      Archivo hosts:
      {#if hostsStatus.upToDate}
        <span class="ok">al día</span>
      {:else}
        <span class="warn">desactualizado</span>
        <button on:click={syncHosts} disabled={syncing}>
          {syncing ? 'Esperando permiso…' : 'Actualizar hosts'}
        </button>
      {/if}
    </p>
    {#if hostsStatus.error}
      <p class="error">{hostsStatus.error}</p>
    {/if}
  {/if}

  {#if ssl}
    <p class="hosts">
      HTTPS:
      {#if ssl.enabled}
        <span class="ok">activado</span>
      {:else}
        <span class="muted">desactivado</span>
      {/if}
      {#if ssl.available}
        <button on:click={toggleSsl} disabled={sslBusy}
          title={ssl.enabled ? 'Volver a solo http://' : 'Windows pedirá confirmar que confías en el certificado de Kiltro'}>
          {#if sslBusy}
            {ssl.enabled ? 'Desactivando…' : 'Esperando confirmación…'}
          {:else}
            {ssl.enabled ? 'Desactivar HTTPS' : 'Activar HTTPS'}
          {/if}
        </button>
      {/if}
    </p>
    {#if ssl.error}
      <p class="muted">{ssl.error}</p>
    {/if}
    {#if sslMsg}
      <p class="error">{sslMsg}</p>
    {/if}
  {/if}
</main>

<style>
  main {
    padding: 2rem;
    text-align: left;
  }

  .root code {
    user-select: all;
  }

  table {
    border-collapse: collapse;
    margin-top: 1rem;
  }

  td {
    padding: 0.4rem 0.8rem;
  }

  .name {
    font-weight: bold;
  }

  .name select {
    margin-left: 0.6rem;
    font-weight: normal;
    padding: 0.2rem 0.3rem;
    border-radius: 3px;
  }

  .ports {
    opacity: 0.6;
  }

  .status {
    padding: 0.1rem 0.5rem;
    border-radius: 3px;
    background: #555;
  }
  .status.running {
    background: #2e7d32;
  }
  .status.starting,
  .status.stopping {
    background: #b28704;
  }
  .status.error {
    background: #c62828;
  }

  h2 {
    margin-top: 2rem;
    font-size: 1.1rem;
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .muted {
    opacity: 0.6;
  }

  .warn {
    color: #ffd54f;
  }

  .ok {
    color: #81c784;
  }

  .hosts {
    margin-top: 1.5rem;
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  a {
    color: inherit;
  }

  .badge {
    padding: 0.1rem 0.5rem;
    border-radius: 3px;
    background: #f4511e;
    font-size: 0.85em;
  }
  .badge.tool {
    background: #5c6bc0;
  }

  .detail {
    margin-left: 0.5rem;
    opacity: 0.7;
    font-size: 0.9em;
  }

  button {
    min-width: 80px;
    padding: 0.3rem 0.8rem;
    border: none;
    border-radius: 3px;
    cursor: pointer;
  }
  button:disabled {
    cursor: wait;
    opacity: 0.6;
  }

  .error {
    color: #ff8a80;
    white-space: pre-wrap;
  }
</style>
