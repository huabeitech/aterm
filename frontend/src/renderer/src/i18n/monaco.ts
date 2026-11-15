import type { SupportedLocale } from '../../../shared/contracts/locale'

export const monacoLoaders = {
  en: () => Promise.resolve(),
  'zh-CN': () => import('monaco-editor/nls/lang/zh-cn.js'),
  'zh-TW': () => import('monaco-editor/nls/lang/zh-tw.js'),
  ja: () => import('monaco-editor/nls/lang/ja.js'),
  'pt-BR': async () => {
    await import('monaco-editor/nls/lang/pt-br.js')
    // Monaco 0.57 ships an empty pt-BR table. Fill its gaps with the versioned
    // VS Code language pack; retain any translations supplied upstream.
    const { default: pack } = await import('./monaco-pt-br.json')
    const nls = globalThis as typeof globalThis & { _VSCODE_NLS_MESSAGES?: (string | null)[] }
    nls._VSCODE_NLS_MESSAGES = pack.messages.map(
      (message, index) => nls._VSCODE_NLS_MESSAGES?.[index] ?? message
    )
  },
  es: () => import('monaco-editor/nls/lang/es.js'),
  de: () => import('monaco-editor/nls/lang/de.js'),
  fr: () => import('monaco-editor/nls/lang/fr.js'),
  ko: () => import('monaco-editor/nls/lang/ko.js')
} satisfies Record<SupportedLocale, () => Promise<unknown>>

export async function loadMonacoLocale(locale: SupportedLocale): Promise<void> {
  await monacoLoaders[locale]()
}
