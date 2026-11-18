import { config, type StaticTextDefaultValue, type ToolbarNames } from 'md-editor-rt'
import { i18n } from '../../../../../../shared/i18n'
import { supportedLocales } from '../../../../../../shared/contracts/locale'
import type hostMessages from '../../../../../../shared/i18n/locales/en/hosts.json'
import DOMPurify from 'dompurify'

export const noteToolbars: ToolbarNames[] = [
  'bold', 'italic', 'strikeThrough', 'title', '-', 'quote', 'unorderedList',
  'orderedList', 'task', '-', 'codeRow', 'code', 'link', 'table',
  '-', 'revoke', 'next', '=', 'preview', 'previewOnly'
]

function editorLanguage(language: string): StaticTextDefaultValue {
  const text = (key: string) => {
    const translationKey = ('markdown' + key[0].toUpperCase() + key.slice(1)) as keyof typeof hostMessages
    return i18n.t(translationKey, { ns: 'hosts', lng: language })
  }
  return {
    toolbarTips: Object.fromEntries(
      noteToolbars.filter((key) => key !== '-' && key !== '=').map((key) => [key, text(String(key))])
    ),
    titleItem: Object.fromEntries(
      [1, 2, 3, 4, 5, 6].map((count) => [
        'h' + count, i18n.t('hosts:markdownHeadingLevel', { lng: language, count })
      ])
    ),
    linkModalTips: {
      linkTitle: text('linkTitle'),
      descLabel: text('descLabel'),
      descLabelPlaceHolder: text('descLabelPlaceHolder'),
      urlLabel: text('urlLabel'),
      urlLabelPlaceHolder: text('urlLabelPlaceHolder'),
      buttonOK: text('buttonOK')
    },
    copyCode: {
      text: text('copy'), successTips: text('copied'), failTips: text('copyFailed')
    }
  }
}

config({
  editorConfig: {
    languageUserDefined: Object.fromEntries(supportedLocales.map((locale) => [locale, editorLanguage(locale)]))
  }
})

export function sanitizeNote(html: string): string {
  return DOMPurify.sanitize(html)
}
