package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"kiltro/internal/config"
	"kiltro/internal/i18n"
	"kiltro/internal/tray"
)

// I18nView es lo que la interfaz necesita para mostrar los textos.
type I18nView struct {
	Lang      string            `json:"lang"`      // idioma en uso: "es" o "en"
	Pref      string            `json:"pref"`      // lo elegido: "" (automático), "es" o "en"
	Supported []string          `json:"supported"` // idiomas disponibles
	Messages  map[string]string `json:"messages"`  // clave → texto en el idioma en uso
}

// GetI18n devuelve el idioma en uso y todos sus textos. La interfaz lo pide al
// arrancar (antes de mostrarse) y cada vez que llega el evento "i18n:changed".
func (a *App) GetI18n() I18nView {
	lang := i18n.Lang()
	return I18nView{
		Lang:      lang,
		Pref:      config.Current().Lang,
		Supported: i18n.Supported,
		Messages:  i18n.Messages(lang),
	}
}

// SetLanguage cambia el idioma ("" = automático, "es", "en"), lo guarda en
// config.json y avisa a la interfaz y a la bandeja. No hace falta reiniciar.
func (a *App) SetLanguage(pref string) (I18nView, error) {
	if pref != "" && i18n.Resolve(pref) != pref {
		return a.GetI18n(), i18n.Err("err.unsupportedLang", "lang", pref)
	}
	if err := config.Update(func(s *config.Settings) { s.Lang = pref }); err != nil {
		return a.GetI18n(), err
	}
	lang := i18n.SetPreference(pref)
	if a.manager != nil {
		a.manager.LogPrintf("[i18n] idioma: %s (elegido: %q)", lang, pref)
	}
	tray.Update(a.trayServices()) // rehace los textos del menú de la bandeja
	runtime.EventsEmit(a.ctx, "i18n:changed")
	return a.GetI18n(), nil
}
