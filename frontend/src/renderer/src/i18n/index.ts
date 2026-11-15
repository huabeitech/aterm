import { resolveLocale } from '../../../shared/i18n/resolveLocale'
import { initializeThemes } from '../app/themeRegistry'
import { i18n } from '../../../shared/i18n'
import { initReactI18next } from 'react-i18next'
import type { LocaleState } from '../../../shared/contracts/locale'

// Initialized before either window is mounted; subscription precedes the snapshot request.
initReactI18next.init(i18n)
let revision = -1
export function applyLocale(state: LocaleState): void {
  if (state.revision < revision) return
  revision = state.revision
  const language = resolveLocale(state.preference, navigator.languages)
  void i18n.changeLanguage(language)
  document.documentElement.lang = language
}
export async function initializeLocale(): Promise<void> {
  window.desktop.onLocaleChanged(applyLocale)
  const bootstrap = await window.desktop.bootstrap()
  applyLocale(bootstrap.locale)
  await initializeThemes()
}
export { i18n }
