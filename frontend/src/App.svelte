<script lang="ts">
  // Sintaxis clásica de Svelte (sin runes), ver CLAUDE.md.
  import { onMount } from 'svelte'
  import { GetAppRoot } from '../wailsjs/go/main/App.js'

  let root: string = '…'

  onMount(async () => {
    root = await GetAppRoot()
  })
</script>

<main>
  <p>Kiltro — raíz: <code>{root}</code></p>
</main>

<style>
  main {
    padding: 2rem;
    text-align: left;
  }

  code {
    user-select: all;
  }
</style>
