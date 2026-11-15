import {
  localeRegistry,
  supportedLocales,
  type LanguagePreference,
  type SupportedLocale
} from '../contracts/locale'
/** Match the ordered OS language list, preserving Chinese script. Regional variants
 * share a reviewed language pack: Portuguese uses Brazilian, Spanish uses neutral Spanish. */
export function resolveLocale(
  preference: LanguagePreference,
  languages: readonly string[]
): SupportedLocale {
  if (preference !== 'system') return preference
  for (const language of languages) {
    try {
      const locale = new Intl.Locale(language.replaceAll('_', '-')).maximize()
      const match = supportedLocales.find((id) => {
        const candidate = localeRegistry[id]
        return (
          candidate.language === locale.language &&
          (!('script' in candidate) || candidate.script === locale.script)
        )
      })
      if (match) return match
    } catch {
      /* Ignore malformed OS locale entries. */
    }
  }
  return 'en'
}
