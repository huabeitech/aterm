import { createInstance, type i18n as I18n, type TFunction } from 'i18next'
import { supportedLocales } from '../contracts/locale'
import { resources } from './resources'

// Each Electron process has its own module graph and instance. Resources are bundled offline.
export const i18n: I18n = createInstance()
void i18n.init({
  resources,
  lng: 'en',
  fallbackLng: 'en',
  supportedLngs: supportedLocales,
  defaultNS: 'common',
  initAsync: false,
  returnNull: false,
  interpolation: { escapeValue: false }
})
export const t: TFunction<(keyof typeof resources.en)[]> = i18n.t.bind(i18n)
