import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { initI18n } from './i18n'

// Primero los textos del idioma elegido (vienen de Go) y recién después se
// muestra la interfaz. (Sin "await" suelto arriba del archivo: la build de Vite
// no lo admite con su configuración por defecto.)
//
// La plantilla de Wails v2 trae Svelte 5, donde `new App(...)` ya no existe;
// `mount` es la forma de montar el componente raíz (no usa runes).
initI18n().finally(() => {
  mount(App, { target: document.getElementById('app')! })
})
