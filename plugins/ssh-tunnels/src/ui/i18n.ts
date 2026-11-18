import locale0 from '../../locales/ja.json'
import locale1 from '../../locales/de.json'
import locale2 from '../../locales/zh-CN.json'
import locale3 from '../../locales/en.json'
import locale4 from '../../locales/fr.json'
import locale5 from '../../locales/pt-BR.json'
import locale6 from '../../locales/ko.json'
import locale7 from '../../locales/zh-TW.json'
import locale8 from '../../locales/es.json'
import { i18n } from '../../../../frontend/src/shared/i18n'
export { i18n }
export const resources = {
  ja: { hosts: locale0 },
  de: { hosts: locale1 },
  'zh-CN': { hosts: locale2 },
  en: { hosts: locale3 },
  fr: { hosts: locale4 },
  'pt-BR': { hosts: locale5 },
  ko: { hosts: locale6 },
  'zh-TW': { hosts: locale7 },
  es: { hosts: locale8 },
}
for (const [locale, resource] of Object.entries(resources))
  i18n.addResourceBundle(
    locale,
    'hosts',
    Object.fromEntries(
      Object.entries(resource.hosts).filter(([key]) => key.startsWith('tunnels.')),
    ),
    true,
    true,
  )
export function t(key: string, options: Record<string, string | number> = {}): string {
  return i18n.t(key as never, options) as string
}
