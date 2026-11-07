// Textos de la interfaz en el idioma elegido.
//
// Los textos NO están aquí: vienen de Go (internal/i18n/locales/*.json), que es
// el único catálogo de Kiltro. Este archivo solo los guarda en un "store" de
// Svelte para que los componentes los usen así:
//
//   {$t('services.title')}                              → "Servicios" / "Services"
//   {$t('sites.created', { domain: 'blog.test' })}      → "Creado blog.test: ..."
//   {@html $th('settings.mysqlPortWarn')}               → textos con <code> adentro
//
// $th es para los textos que traen HTML (solo <code>, <strong>...). Los valores
// que se insertan ({dir}, {folder}...) se escapan, así un nombre de carpeta raro
// nunca se interpreta como HTML.
//
// Al cambiar el idioma, Go emite "i18n:changed": se vuelven a pedir los textos y
// todo lo que usa $t se actualiza solo.
import { writable, derived, get } from 'svelte/store'
import { GetI18n, SetLanguage } from '../wailsjs/go/main/App.js'
import { EventsOn } from '../wailsjs/runtime/runtime.js'

type Params = Record<string, string | number>

export const lang = writable('es')       // idioma en uso
export const langPref = writable('')     // lo elegido: '' = automático
export const supported = writable<string[]>(['es', 'en'])
const messages = writable<Record<string, string>>({})

function fill(text: string, params: Params | undefined, escape: boolean): string {
  if (!params) return text
  return text.replace(/\{([a-zA-Z]+)\}/g, (whole, name) => {
    if (!(name in params)) return whole
    const v = String(params[name])
    return escape ? escapeHtml(v) : v
  })
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

// $t('clave', {valores}): texto plano. Si falta la clave, se muestra la clave.
export const t = derived(messages, (m) => (key: string, params?: Params) => fill(m[key] ?? key, params, false))

// $th('clave', {valores}): texto con HTML, para usar con {@html ...}.
export const th = derived(messages, (m) => (key: string, params?: Params) => fill(m[key] ?? key, params, true))

async function load() {
  const v = await GetI18n()
  messages.set(v.messages ?? {})
  lang.set(v.lang)
  langPref.set(v.pref ?? '')
  supported.set(v.supported ?? ['es', 'en'])
  document.documentElement.lang = v.lang
}

// initI18n se llama una vez en main.ts, ANTES de montar la app (así nunca se
// ven las claves en vez de los textos).
export async function initI18n() {
  await load()
  EventsOn('i18n:changed', load)
}

// changeLanguage: '' = automático (el de Windows), 'es' o 'en'.
export async function changeLanguage(pref: string) {
  await SetLanguage(pref)
  await load() // el evento también llega, pero así el selector se actualiza enseguida
}

// Para usar fuera de los componentes (en funciones normales).
export function tr(key: string, params?: Params): string {
  return get(t)(key, params)
}
