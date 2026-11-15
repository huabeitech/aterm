import messages from './i18n/locales/en/errors.json' with { type: 'json' }
export type ErrorCode = keyof typeof messages
export type ErrorParams = Record<string, string | number>
export interface ErrorPayload {
  code: ErrorCode | 'unexpected'
  params?: ErrorParams
  detail?: string
}
export interface ErrorResponse {
  __atermError: ErrorPayload
}
/** Domain errors retain diagnostic English; presentation layers translate the stable code. */
export class AppError extends Error {
  constructor(
    readonly code: ErrorCode,
    readonly params: ErrorParams = {}
  ) {
    super(messages[code].replace(/{{(\w+)}}/g, (_, name: string) => String(params[name] ?? '')))
    this.name = 'AppError'
  }
}
export const appError = (code: ErrorCode, params?: ErrorParams): AppError =>
  new AppError(code, params)
export function serializeError(error: unknown): ErrorResponse {
  return {
    __atermError:
      error instanceof AppError
        ? { code: error.code, params: error.params }
        : { code: 'unexpected', detail: error instanceof Error ? error.message : String(error) }
  }
}
export function isErrorResponse(value: unknown): value is ErrorResponse {
  return !!value && typeof value === 'object' && '__atermError' in value
}

// contextBridge copies Error.message but drops custom fields. Carry the already structured
// IPC payload in a versioned suffix, retaining a readable diagnostic for direct API callers.
const wireMarker = '\n[aterm-error-v1]'
export function bridgeError(payload: ErrorPayload): Error {
  const diagnostic =
    payload.code === 'unexpected'
      ? (payload.detail ?? messages.unexpected)
      : new AppError(payload.code, payload.params).message
  return new Error(diagnostic + wireMarker + JSON.stringify(payload))
}
export function readBridgeError(error: unknown): ErrorPayload | undefined {
  if (!(error instanceof Error)) return undefined
  const offset = error.message.lastIndexOf(wireMarker)
  if (offset < 0) return undefined
  try {
    const payload = JSON.parse(error.message.slice(offset + wireMarker.length)) as ErrorPayload
    if (payload && (payload.code === 'unexpected' || Object.hasOwn(messages, payload.code)))
      return payload
  } catch {
    /* Non-application failures use the ordinary diagnostic. */
  }
  return undefined
}
