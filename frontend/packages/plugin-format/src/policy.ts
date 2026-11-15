import type { PluginExecutionPolicy } from '../../plugin-sdk'
const programName = /^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,63}$/
const envName = /^[A-Z_][A-Z0-9_]{0,63}$/
function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw Error('Invalid capability request')
  return value as Record<string, unknown>
}
export function validateExecutionPolicy(input: unknown): PluginExecutionPolicy {
  const p = object(input)
  if (Object.keys(p).some((k) => !['programs', 'targets', 'unsetEnv', 'env'].includes(k)))
    throw Error('Unknown execution policy field')
  const list = (value: unknown, max: number, valid: (v: string) => boolean) => {
    if (
      !Array.isArray(value) ||
      !value.length ||
      value.length > max ||
      value.some((v) => typeof v !== 'string' || !valid(v)) ||
      new Set(value).size !== value.length
    )
      throw Error('Invalid execution policy')
    return (value as string[]).slice().sort()
  }
  const programs = list(p.programs, 16, (v) => programName.test(v))
  const targets = list(p.targets, 2, (v) =>
    ['local', 'ssh'].includes(v)
  ) as PluginExecutionPolicy['targets']
  const unsetEnv =
    p.unsetEnv === undefined ? undefined : list(p.unsetEnv, 16, (v) => envName.test(v))
  const env = p.env === undefined ? undefined : list(p.env, 16, (v) => envName.test(v))
  return { programs, targets, ...(unsetEnv ? { unsetEnv } : {}), ...(env ? { env } : {}) }
}
