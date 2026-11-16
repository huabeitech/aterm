import type english from '../../locales/en.json'
export type MessageKey = keyof typeof english
/** Runtime language selection, fallback and interpolation belong to the public plugin SDK. */
export function translator(_locale: string) {
  return (key: MessageKey, values: Record<string, string | number> = {}) => atermView.t(key, values)
}
export type Translate = ReturnType<typeof translator>

export function executionError(message: string, operation: string) {
  if (message === 'HOST_UPDATE_REQUIRED') return atermView.t('hostUpdateRequired')
  if (/Execution output limit exceeded|Plugin data limit/.test(message))
    return atermView.t(
      operation === 'logs'
        ? 'logsTooLarge'
        : operation === 'list' || operation === 'images.list'
          ? 'listTooLarge'
          : 'detailTooLarge'
    )
  return message
}
