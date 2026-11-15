import registry from '../i18n/locales.json' with { type: 'json' }
export type SupportedLocale = keyof typeof registry
export const localeRegistry = registry
export const supportedLocales = Object.keys(registry) as SupportedLocale[]
export const languageOptions = supportedLocales.map((value) => ({
  value,
  label: registry[value].nativeName
}))
export type LanguagePreference = 'system' | SupportedLocale
export interface LocaleState {
  preference: LanguagePreference
  language: SupportedLocale
  revision: number
}
export function isSupportedLocale(value: unknown): value is SupportedLocale {
  return typeof value === 'string' && Object.hasOwn(registry, value)
}
export function isLanguagePreference(value: unknown): value is LanguagePreference {
  return value === 'system' || isSupportedLocale(value)
}
