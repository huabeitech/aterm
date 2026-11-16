import { dockerDeadline } from './deadline'
import type { JsonValue, PluginApi, PluginViewAvailability } from '../../../frontend/packages/plugin-sdk'
import type { DockerClient } from './client'
import { dockerArgs, dockerRequest, verifyEngine } from './operations'
import { LogBuffer, logLimits } from './logs'
interface Session {
  subscription: string
  snapshot: JsonValue
  dispose?: () => Promise<void>
}
/** Docker owns CLI semantics and view data; the host only transports authorized bytes. */
export class DockerLogStreams {
  private sessions = new Map<string, Session>()
  constructor(
    private api: PluginApi,
    private client: DockerClient
  ) {}
  async publish(token: string, views?: Record<string, PluginViewAvailability>) {
    const session = this.sessions.get(token)
    const result = await this.api.request('status.publish', {
      token,
      state: 'ready',
      items: [],
      ...(views ? { views } : {}),
      ...(session ? { data: { dockerLogs: session.snapshot } } : {})
    })
    if (!result.ok) {
      this.sessions.delete(token)
      await session?.dispose?.()
      throw Error('Log subscription unavailable')
    }
  }
  async command(token: string, input: JsonValue): Promise<JsonValue> {
    if (
      !input ||
      typeof input !== 'object' ||
      Array.isArray(input) ||
      Object.keys(input).some(
        (k) => !['action', 'subscription', 'id', 'context', 'engine'].includes(k)
      ) ||
      typeof input.subscription !== 'string' ||
      !/^[a-zA-Z0-9_-]{1,80}$/.test(input.subscription) ||
      typeof input.action !== 'string' ||
      !['start', 'stop'].includes(input.action)
    )
      throw Error('Invalid log subscription')
    const subscription = input.subscription
    const previous = this.sessions.get(token)
    if (input.action === 'stop') {
      if (previous?.subscription === subscription) {
        this.sessions.delete(token)
        await previous.dispose?.()
      }
      return null
    }
    if (!this.api.capabilities.processStreaming) throw Error('HOST_UPDATE_REQUIRED')
    const request = dockerRequest({
      operation: 'logs',
      id: input.id,
      context: input.context,
      engine: input.engine
    })
    this.sessions.delete(token)
    await previous?.dispose?.()
    const deadline = dockerDeadline(30000)
    await verifyEngine(
      request,
      async (args) => {
        const result = await this.client.run({ token, executable: 'docker', args }, deadline)
        if (result.exitCode !== 0) throw Error(result.stderr.trim() || 'Docker engine unavailable')
        return result.stdout
      },
      deadline
    )
    const buffer = new LogBuffer()
    const session: Session = {
      subscription,
      snapshot: { subscription, id: request.id!, text: '', truncated: false, ended: false }
    }
    this.sessions.set(token, session)
    try {
      const stream = await this.api.process.subscribe(
        {
          token,
          executable: 'docker',
          env: await this.client.environment(token, request.context, deadline),
          args: [...dockerArgs(request).slice(0, -1), '--follow', request.id!],
          maxOutputBytes: logLimits.bytes
        },
        async (event) => {
          if (this.sessions.get(token) !== session) return
          const error =
            event.error ||
            (event.done && event.exitCode !== 0 ? `Docker exited with code ${event.exitCode}` : '')
          session.snapshot = {
            subscription,
            id: request.id!,
            ...buffer.append(event.text, event.truncated),
            ended: event.done,
            ...(error ? { error } : {})
          }
          await this.publish(token)
          if (event.done && this.sessions.get(token) === session) this.sessions.delete(token)
        }
      )
      session.dispose = () => stream.dispose()
      return null
    } catch (error) {
      if (this.sessions.get(token) === session) this.sessions.delete(token)
      throw error
    }
  }
}
