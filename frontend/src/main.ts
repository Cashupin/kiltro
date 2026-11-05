import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'

// La plantilla de Wails v2 trae Svelte 5, donde `new App(...)` ya no existe;
// `mount` es la forma de montar el componente raíz (no usa runes).
const app = mount(App, {
  target: document.getElementById('app')!
})

export default app
