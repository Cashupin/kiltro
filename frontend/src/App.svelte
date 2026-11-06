<script lang="ts">
  // Sintaxis clásica de Svelte (sin runes), ver CLAUDE.md.
  import { onMount, onDestroy } from 'svelte'
  import {
    GetAppRoot, GetServices, StartService, StopService, GetSites, ReloadSites,
  } from '../wailsjs/go/main/App.js'
  import { EventsOn } from '../wailsjs/runtime/runtime.js'
  import type { services, vhosts } from '../wailsjs/go/models'

  let root: string = '…'
  let list: services.Info[] = []
  let errorMsg: string = ''         // error general (ej. puerto ocupado)
  let busy: Record<string, boolean> = {} // servicio con una acción en curso

  let sites: vhosts.Site[] = []
  let skipped: vhosts.Skipped[] = []
  let reloading = false
  let sitesMsg = '' // resultado de "Recargar sitios"

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

  // [9000, 9001, 9002, 9003] → ":9000-9003"; [80] → ":80"
  function portsLabel(ports: number[]): string {
    if (!ports || ports.length === 0) return ''
    if (ports.length === 1) return ':' + ports[0]
    return ':' + ports[0] + '-' + ports[ports.length - 1]
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
  let off: () => void = () => {}

  onMount(async () => {
    root = await GetAppRoot()
    off = EventsOn('services:changed', refresh)
    await refresh()
    await refreshSites()
  })

  onDestroy(() => off())
</script>

<main>
  <p class="root">Kiltro — raíz: <code>{root}</code></p>

  <table>
    <tbody>
      {#each list as s (s.name)}
        <tr>
          <td class="name">{names[s.name] ?? s.name}</td>
          <td class="ports">{portsLabel(s.ports)}</td>
          <td>
            <span class="status {s.status}">{labels[s.status] ?? s.status}</span>
            {#if s.detail}<span class="detail">{s.detail}</span>{/if}
          </td>
          <td>
            <button on:click={() => toggle(s)} disabled={busy[s.name]}>
              {s.status === 'running' || s.status === 'starting' ? 'Detener' : 'Iniciar'}
            </button>
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
            <td class="name">{site.domain}</td>
            <td>{#if site.laravel}<span class="badge">Laravel</span>{/if}</td>
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

  .badge {
    padding: 0.1rem 0.5rem;
    border-radius: 3px;
    background: #f4511e;
    font-size: 0.85em;
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
