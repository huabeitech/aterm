import { dockerDeadline } from './deadline'
import type { JsonValue } from '../../../frontend/packages/plugin-sdk'
import { dockerArgs, dockerRequest, runDocker } from './operations'
import { DockerClient } from './client'
import { EngineVerification } from './engineVerification'
import { logLimits } from './logs'
import { DockerLogStreams } from './logStream'
import { DockerAvailabilityCache } from './availability'
const verification = new EngineVerification()
const api = atermPlugin
const client = new DockerClient(api)
const logStreams = new DockerLogStreams(api, client)
const availability = new DockerAvailabilityCache()
api.onRefresh(async ({ token }) => {
  const state = await availability.check(token, (args, timeoutMs) =>
    client.run({
      token,
      executable: 'docker',
      args,
      timeoutMs,
      maxOutputBytes: 4096
    })
  )
  const message = state.reason
    ? api.i18n.t(
        {
          missing: 'missing',
          permission: 'availabilityPermission',
          daemon: 'availabilityDaemon',
          unknown: 'availabilityUnknown'
        }[state.reason]
      )
    : undefined
  await logStreams.publish(token, {
    'docker.containers': {
      visible: state.visible,
      available: state.available,
      ...(message ? { message } : {})
    }
  })
})
api.commands.registerCommand('docker.logs', async ({ token, args }) => {
  try {
    return await logStreams.command(token, args)
  } catch (error) {
    return { error: error instanceof Error ? error.message : 'Log subscription failed' }
  }
})
api.commands.registerCommand('docker.request', async ({ token, args }): Promise<JsonValue> => {
  try {
    const request = dockerRequest(args)
    if (request.operation === 'connect') client.reset(token)
    if (request.operation === 'logs' && !api.capabilities.processOutputTail)
      throw Error('HOST_UPDATE_REQUIRED')
    let truncated = false
    const deadline = dockerDeadline(30000)
    const result = await runDocker(
      request,
      async (args, signal) => {
        signal.throwIfAborted()
        const output = await client.run({
          token,
          executable: 'docker',
          args,
          timeoutMs: deadline.remaining(),
          ...(args.includes('logs')
            ? { output: 'tail' as const, maxOutputBytes: logLimits.bytes }
            : {})
        }, deadline)
        truncated ||= output.truncated === true
        if (output.exitCode !== 0)
          throw Error(output.stderr.trim() || `Docker exited with code ${output.exitCode}`)
        return output.stdout + (args.includes('logs') ? output.stderr : '')
      },
      deadline,
      {
        fresh: () => verification.fresh(token, request.context!, request.engine!),
        remember: (context, engine) => verification.remember(token, context, engine)
      }
    )
    if (request.operation === 'exec') {
      return await api.terminals.open({
        token,
        executable: 'docker',
        env: await client.environment(token, request.context, deadline),
        args: dockerArgs(request),
        title: `Docker · ${request.id!.slice(0, 12)}`,
        checks: [
          {
            args: ['--context', request.context!, 'info', '--format', '{{.ID}}'],
            stdout: request.engine!
          }
        ]
      })
    }
    return { data: request.operation === 'logs' ? { text: result, truncated } : result }
  } catch (error) {
    return {
      error: error instanceof Error ? error.message : 'Docker operation failed',
      operation:
        (args && typeof args === 'object' && !Array.isArray(args) ? args.operation : null) ?? null
    }
  }
})
void api.request('runtime.ready', {})
