import type { DockerDeadline } from './deadline'
import { logLimits } from './logs'
import { imageUsage } from './imageUsage'
import type { JsonValue } from '../../../frontend/packages/plugin-sdk'
/** Docker CLI semantics belong to this plugin, never to the application host. */
export type DockerOperation =
  | 'connect'
  | 'list'
  | 'inspect'
  | 'logs'
  | 'start'
  | 'stop'
  | 'restart'
  | 'remove'
  | 'exec'
  | 'images.list'
  | 'images.inspect'
  | 'images.remove'
export interface DockerRequest {
  operation: DockerOperation
  id?: string
  context?: string
  engine?: string
  shell?: 'sh' | 'bash'
}
const operations = new Set([
  'connect',
  'list',
  'inspect',
  'logs',
  'start',
  'stop',
  'restart',
  'remove',
  'exec',
  'images.list',
  'images.inspect',
  'images.remove'
])
export function dockerRequest(input: unknown): DockerRequest {
  if (!input || typeof input !== 'object' || Array.isArray(input))
    throw Error('Invalid Docker request')
  const r = input as DockerRequest
  if (
    !operations.has(r.operation) ||
    Object.keys(r).some((k) => !['operation', 'id', 'context', 'engine', 'shell'].includes(k))
  )
    throw Error('Invalid Docker operation')
  if (
    r.operation !== 'connect' &&
    (typeof r.context !== 'string' ||
      !/^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,199}$/.test(r.context) ||
      typeof r.engine !== 'string' ||
      !r.engine ||
      r.engine.length > 200)
  )
    throw Error('Reconnect to the Docker engine first')
  if (
    !['connect', 'list', 'images.list'].includes(r.operation) &&
    (typeof r.id !== 'string' ||
      !(
        r.operation.startsWith('images.') ? /^(?:sha256:)?[a-f0-9]{12,64}$/ : /^[a-f0-9]{12,64}$/
      ).test(r.id))
  )
    throw Error('Invalid container ID')
  if (r.shell !== undefined && !['sh', 'bash'].includes(r.shell))
    throw Error('Invalid container shell')
  return r
}
// Project in the Docker CLI before crossing the plugin bridge; omit large unused metadata.
export const containerProjection = `{"Id":{{json .Id}},"Name":{{json .Name}},"Created":{{json .Created}},"State":{"Status":{{json .State.Status}},"Health":{{with index .State "Health"}}{"Status":{{json .Status}}}{{else}}null{{end}}},"Config":{"Image":{{json .Config.Image}}},"HostConfig":{"RestartPolicy":{{json .HostConfig.RestartPolicy}}},"Mounts":[{{range $i,$m := .Mounts}}{{if $i}},{{end}}{"Source":{{json $m.Source}},"Destination":{{json $m.Destination}},"RW":{{json $m.RW}}}{{end}}],"NetworkSettings":{"Ports":{{json .NetworkSettings.Ports}},"Networks":{ {{$first := true}}{{range $name,$network := .NetworkSettings.Networks}}{{if not $first}},{{end}}{{$first = false}}{{json $name}}:{}{{end}} }}}`
export const imageProjection = `{"Id":{{json .Id}},"RepoTags":{{json .RepoTags}},"RepoDigests":{{json .RepoDigests}},"Created":{{json .Created}},"Size":{{json .Size}},"Os":{{json .Os}},"Architecture":{{json .Architecture}},"LayersCount":{{len .RootFS.Layers}},"Config":{"Entrypoint":{{json .Config.Entrypoint}},"Cmd":{{json .Config.Cmd}}}}`
export const infoProjection = `{"ID":{{json .ID}},"Name":{{json .Name}},"ServerVersion":{{json .ServerVersion}}}`
export function isMutation(operation: DockerOperation) {
  return ['start', 'stop', 'restart', 'remove', 'exec', 'images.remove'].includes(operation)
}
export function dockerArgs(r: DockerRequest): string[] {
  const base = ['--context', r.context!]
  switch (r.operation) {
    case 'images.list':
      return [...base, 'image', 'ls', '--no-trunc', '--format', '{{json .}}']
    case 'images.inspect':
      return [...base, 'image', 'inspect', '--format', imageProjection, r.id!]
    case 'images.remove':
      return [...base, 'image', 'rm', r.id!]
    case 'list':
      return [
        ...base,
        'ps',
        '-a',
        '--no-trunc',
        '--format',
        '{"ID":{{json .ID}},"Names":{{json .Names}},"Image":{{json .Image}},"State":{{json .State}},"Status":{{json .Status}},"Ports":{{json .Ports}}}'
      ]
    case 'inspect':
      return [...base, 'inspect', '--type', 'container', '--format', containerProjection, r.id!]
    case 'logs':
      return [...base, 'logs', '--timestamps', '--tail', String(logLimits.lines), r.id!]
    case 'remove':
      return [...base, 'rm', r.id!]
    case 'exec':
      return [...base, 'exec', '-it', r.id!, r.shell ?? 'sh']
    case 'start':
      return [...base, 'start', r.id!]
    case 'stop':
    case 'restart':
      return [...base, r.operation, '--time', '10', r.id!]
    default:
      throw Error('Invalid Docker operation')
  }
}
export type DockerExecutor = (args: string[], signal: DockerDeadline) => Promise<string>
/** Recheck engine identity immediately before starting a persistent read or a mutation. */
export async function verifyEngine(
  r: DockerRequest,
  execute: (args: string[], signal: DockerDeadline) => Promise<string>,
  signal: DockerDeadline
) {
  const engine = (
    await execute(['--context', r.context!, 'info', '--format', '{{.ID}}'], signal)
  ).trim()
  if (engine !== r.engine) throw Error('Docker engine changed. Reconnect before continuing.')
}
export async function runDocker(
  r: DockerRequest,
  execute: DockerExecutor,
  signal: DockerDeadline,
  verification?: { fresh: () => boolean; remember: (context: string, engine: string) => void }
): Promise<JsonValue> {
  if (r.operation === 'connect') {
    const context = (await execute(['context', 'show'], signal)).trim()
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,199}$/.test(context)) throw Error('Invalid Docker context')
    const info = JSON.parse(
      await execute(['--context', context, 'info', '--format', infoProjection], signal)
    )
    const endpoint = JSON.parse(
      await execute(
        ['context', 'inspect', context, '--format', '{{json .Endpoints.docker.Host}}'],
        signal
      )
    )
    verification?.remember(context, info.ID)
    return { context, engine: info.ID, name: info.Name, version: info.ServerVersion, endpoint }
  }
  if (isMutation(r.operation) || !verification?.fresh()) {
    await verifyEngine(r, execute, signal)
    verification?.remember(r.context!, r.engine!)
  }
  if (r.operation === 'exec') return null
  const output = await execute(dockerArgs(r), signal)
  if (r.operation === 'images.list') {
    const images = new Map<
      string,
      { ID: string; RepoTags: string[]; CreatedAt: string; Size: string }
    >()
    for (const line of output.trim().split('\n').filter(Boolean)) {
      const row = JSON.parse(line)
      if (typeof row.ID !== 'string' || !/^(?:sha256:)?[a-f0-9]{12,64}$/.test(row.ID))
        throw Error('Invalid image ID from Docker')
      const image = images.get(row.ID) ?? {
        ID: row.ID,
        RepoTags: [] as string[],
        CreatedAt: row.CreatedAt ?? '',
        Size: row.Size ?? ''
      }
      if (row.Repository && row.Repository !== '<none>' && row.Tag && row.Tag !== '<none>') {
        const tag = `${row.Repository}:${row.Tag}`
        if (!image.RepoTags.includes(tag)) image.RepoTags.push(tag)
      }
      images.set(row.ID, image)
    }
    if (!images.size) return []
    const usage = await imageUsage(r.context!, execute, signal)
    return [...images.values()].map((image) => ({ ...image, Containers: usage(image.ID) }))
  }
  if (r.operation === 'images.inspect') {
    const image = JSON.parse(output)
    return {
      Id: image.Id ?? null,
      RepoTags: image.RepoTags ?? [],
      RepoDigests: image.RepoDigests ?? [],
      Created: image.Created ?? null,
      Size: image.Size ?? 0,
      Os: image.Os ?? null,
      Architecture: image.Architecture ?? null,
      Layers: image.LayersCount ?? image.RootFS?.Layers?.length ?? 0,
      Entrypoint: image.Config?.Entrypoint ?? [],
      Cmd: image.Config?.Cmd ?? []
    }
  }
  if (r.operation === 'list')
    return output
      .trim()
      .split('\n')
      .filter(Boolean)
      .map((line) => JSON.parse(line))
  if (r.operation === 'inspect') {
    const c = JSON.parse(output)
    // Do not expose environment variables or unrelated daemon internals by default.
    return {
      Id: c.Id ?? null,
      Name: c.Name ?? null,
      Created: c.Created ?? null,
      State: c.State ?? null,
      Image: c.Config?.Image ?? null,
      RestartPolicy: c.HostConfig?.RestartPolicy ?? null,
      Ports: c.NetworkSettings?.Ports ?? null,
      Mounts: c.Mounts ?? [],
      Networks: Object.keys(c.NetworkSettings?.Networks ?? {})
    }
  }
  return output
}
