import type { PluginProcessResult } from '../../../frontend/packages/plugin-sdk'
type Execute = (args: string[], timeoutMs: number) => Promise<PluginProcessResult>
export interface DockerAvailability {
  visible: boolean
  available: boolean
  reason?: 'missing' | 'permission' | 'daemon' | 'unknown'
}
const permission = (message: string) => /permission denied|access denied/i.test(message)
const missing = (message: string) =>
  /Executable not found: docker/.test(message) ||
  (/docker/i.test(message) && /not found|No such file|not recognized/i.test(message))
/** Only a confirmed missing CLI hides the tab. Daemon/socket failures keep a diagnostic entry. */
export async function probeDocker(execute: Execute): Promise<DockerAvailability> {
  try {
    const client = await execute(['--version'], 2000)
    if (client.exitCode !== 0)
      return {
        visible: !(client.exitCode === 127 && missing(client.stderr)),
        available: false,
        reason:
          client.exitCode === 127 && missing(client.stderr)
            ? 'missing'
            : permission(client.stderr)
              ? 'permission'
              : 'unknown'
      }
  } catch (error) {
    const absent = missing(String((error as Error).message))
    return {
      visible: !absent,
      available: false,
      reason: absent
        ? 'missing'
        : permission(String((error as Error).message))
          ? 'permission'
          : 'unknown'
    }
  }
  try {
    const engine = await execute(['info', '--format', '{{.ID}}'], 3000)
    if (engine.exitCode === 0 && engine.stdout.trim()) return { visible: true, available: true }
    return {
      visible: true,
      available: false,
      reason: permission(engine.stderr) ? 'permission' : 'daemon'
    }
  } catch {
    return { visible: true, available: false, reason: 'daemon' }
  }
}
export class DockerAvailabilityCache {
  private entries = new Map<string, { expires: number; value: DockerAvailability }>()
  constructor(private now = Date.now) {}
  async check(token: string, execute: Execute) {
    const cached = this.entries.get(token)
    if (cached && cached.expires > this.now()) return cached.value
    const value = await probeDocker(execute)
    if (this.entries.size >= 64) this.entries.clear()
    this.entries.set(token, { expires: this.now() + 15000, value })
    return value
  }
}
