import type { DockerBudget } from './deadline'
import type { PluginApi, PluginProcessRequest, PluginProcessResult } from '../../../frontend/packages/plugin-sdk'

type Environment = Record<string, string | null>
const automatic: Environment = { DOCKER_API_VERSION: null }
const projection = '{"ID":{{json .ID}}}'

/** Negotiate with read-only probes; never retry a caller's command (which may mutate). */
export class DockerClient {
  private entries = new Map<string, { expires: number; pending: Promise<Environment> }>()
  constructor(
    private api: PluginApi,
    private now = Date.now
  ) {}

  reset(token: string) {
    for (const key of this.entries.keys()) {
      if (JSON.parse(key)[0] === token) this.entries.delete(key)
    }
  }

  async environment(token: string, context?: string, budget?: DockerBudget): Promise<Environment> {
    const key = JSON.stringify([token, context ?? ''])
    const cached = this.entries.get(key)
    if (cached && cached.expires > this.now()) return cached.pending
    const pending = this.negotiate(token, context, budget)
    const entry = { expires: this.now() + 15000, pending }
    if (this.entries.size >= 64) this.entries.clear()
    this.entries.set(key, entry)
    try {
      return await pending
    } catch (error) {
      if (this.entries.get(key) === entry) this.entries.delete(key)
      throw error
    }
  }

  private async negotiate(token: string, context?: string, budget?: DockerBudget): Promise<Environment> {
    let env = automatic
    const attempted = new Set<string>()
    for (let attempt = 0; attempt < 3; attempt++) {
      const result = await this.api.process.run({
        token,
        executable: 'docker',
        env,
        args: [...(context ? ['--context', context] : []), 'info', '--format', projection],
        timeoutMs: Math.min(3000, budget?.remaining() ?? 3000),
        maxOutputBytes: 4096
      })
      if (result.exitCode === 0) {
        const info = JSON.parse(result.stdout)
        if (typeof info.ID === 'string' && info.ID) return env
        if (!result.stderr.trim()) throw Error('Invalid Docker engine identity')
      }
      // Match only explicit API mismatch diagnostics, never permissions or connection errors.
      const message = result.stderr.trim()
      const mismatch = /client version \d+\.\d+ is too (?:new|old)/i.test(message)
      const version = mismatch
        ? /(?:maximum supported API version is|minimum supported API version is)\s*(\d+\.\d+)/i.exec(
            message
          )?.[1]
        : undefined
      if (!version || attempted.has(version))
        throw Error(message || `Docker exited with code ${result.exitCode}`)
      attempted.add(version)
      env = { DOCKER_API_VERSION: version }
    }
    throw Error('Docker client and server have no usable API version')
  }

  async run(request: PluginProcessRequest & { token: string }, budget?: DockerBudget): Promise<PluginProcessResult> {
    const contextIndex = request.args.indexOf('--context')
    const context = contextIndex < 0 ? undefined : request.args[contextIndex + 1]
    // These commands do not contact the daemon and must work even when it is unavailable.
    const local = request.args[0] === '--version' || request.args[0] === 'context'
    const env = local ? automatic : await this.environment(request.token, context, budget)
    const execute = (environment: Environment) =>
      this.api.process.run({
        ...request,
        env: environment,
        ...(budget && { timeoutMs: Math.min(request.timeoutMs ?? 30000, budget.remaining()) })
      })
    const result = await execute(env)
    if (/client version \d+\.\d+ is too (?:new|old)/i.test(result.stderr)) {
      this.entries.delete(JSON.stringify([request.token, context ?? '']))
      const read = request.args.some((arg) => ['ps', 'info', 'inspect', 'ls', 'logs'].includes(arg))
      const mutation = request.args.some((arg) =>
        ['start', 'stop', 'restart', 'rm', 'exec'].includes(arg)
      )
      if (!local && read && !mutation) {
        const next = await this.environment(request.token, context, budget)
        return execute(next)
      }
    }
    return result
  }
}
