import './wailsDesktop'
import { readBridgeError } from '../../../shared/errors'
import { t } from '../../../shared/i18n'

/** Decode IPC error envelopes here, after crossing contextBridge (which strips Error properties). */
function localizeApi<T extends object>(api: T): T {
  return Object.fromEntries(
    Object.entries(api).map(([name, value]) => [
      name,
      typeof value === 'function'
        ? (...args: unknown[]) => {
            const result: unknown = value(...args)
            if (!(result instanceof Promise)) return result
            return result.catch((cause: unknown) => {
              const payload = readBridgeError(cause)
              if (!payload) throw cause
              const { code, params, detail } = payload
              const error = new Error(
                code === 'unexpected'
                  ? detail
                    ? t('errors:unexpectedWithDetail', { detail })
                    : t('errors:unexpected')
                  : t(`errors:${code}`, params ?? {})
              )
              Object.assign(error, { code, detail })
              throw error
            })
          }
        : value && typeof value === 'object'
          ? localizeApi(value)
          : value
    ])
  ) as T
}
export const desktop = localizeApi(window.desktop)
