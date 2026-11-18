import { t } from './i18n'
import { resources } from './i18n'

type TunnelKey = Extract<keyof typeof resources.en.hosts, `tunnels.${string}`>

// Re-resolve stored validation messages when the language changes. Raw network
// diagnostics remain available, wrapped in a translated explanation.
export function tunnelErrorMessage(value: unknown): string {
  const detail = (value instanceof Error ? value.message : String(value)).replace(/^Error: /, '')
  for (const locale of Object.values(resources)) {
    for (const [key, message] of Object.entries(locale.hosts)) {
      if (key.startsWith('tunnels.') && message === detail) return t(`hosts:${key as TunnelKey}`)
    }
  }
  if (/^(local listener:|listen on )/.test(detail))
    return t('hosts:tunnels.localFailed', { detail })
  if (/^(SSH connection:|connect SSH host:)/.test(detail))
    return t('hosts:tunnels.sshFailed', { detail })
  if (/^(target service:|connect target )/.test(detail))
    return t('hosts:tunnels.targetFailed', { detail })
  return t('hosts:tunnels.operationFailed', { detail })
}
