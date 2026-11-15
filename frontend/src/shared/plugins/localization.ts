import type { PluginLocalizations, PluginManifest } from '../contracts/plugins'
export type PluginTranslate = (key: string, options?: Record<string, string | number>) => string
/** Self-contained factory shared by the host renderer and the sandbox SDK. No App resources. */
export function createPluginTranslator(
  bundle: PluginLocalizations | undefined,
  locale: string
): PluginTranslate {
  const fallback = bundle?.defaultLocale ?? 'en'
  let language = locale
  try {
    language = Intl.getCanonicalLocales(locale.replaceAll('_', '-'))[0] ?? fallback
  } catch {
    language = fallback
  }
  const languages = [...new Set([language, language.split('-')[0], fallback])]
  return (key, options = {}) => {
    for (const candidate of languages) {
      const messages = bundle?.messages[candidate]
      if (!messages) continue
      const variants = [key]
      if (typeof options.count === 'number' && Number.isFinite(options.count)) {
        const category = new Intl.PluralRules(candidate).select(options.count)
        variants.unshift(`${key}_${category}`, `${key}_other`)
      }
      for (const variant of variants) {
        if (!Object.hasOwn(messages, variant)) continue
        return messages[variant].replace(/{{\s*([\w.-]+)\s*}}/g, (match, name: string) =>
          Object.hasOwn(options, name) ? String(options[name]) : match
        )
      }
    }
    return key
  }
}
export function pluginDisplayName(
  manifest: PluginManifest,
  bundle: PluginLocalizations | undefined,
  locale: string
) {
  const name = createPluginTranslator(bundle, locale)('name')
  return (name === 'name' ? manifest.name : name).slice(0, 160)
}
