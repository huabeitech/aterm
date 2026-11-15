import type { domain } from '../../../api/types'
export function buildSSHCommand(host: domain.Host, hosts: domain.Host[]) {
  const byID = new Map(hosts.map((item) => [item.id, item]))
  const jumps: string[] = []
  const visited = new Set<string>([host.id])
  let current = host
  while (current.jumpHostID) {
    const jump = byID.get(current.jumpHostID)
    if (!jump || visited.has(jump.id)) break
    visited.add(jump.id)
    jumps.unshift(jumpEndpoint(jump))
    current = jump
  }
  const args = ['ssh']
  // aterm-key:// is an application-private reference, not a path understood by OpenSSH.
  // Managed keys can be made available to OpenSSH through ssh-agent or an explicit export.
  if (host.identityFile && !host.identityFile.startsWith('aterm-key://'))
    args.push('-i', shellQuote(host.identityFile))
  if (host.port && host.port !== 22) args.push('-p', String(host.port))
  if (jumps.length) args.push('-J', shellQuote(jumps.join(',')))
  if (host.forwardAgent) args.push('-A')
  if (host.compression) args.push('-C')
  if (host.serverAliveInterval > 0)
    args.push('-o', `ServerAliveInterval=${host.serverAliveInterval}`)
  if (host.serverAliveCountMax > 0)
    args.push('-o', `ServerAliveCountMax=${host.serverAliveCountMax}`)
  if (host.connectTimeout > 0) args.push('-o', `ConnectTimeout=${host.connectTimeout}`)
  args.push(shellQuote(`${host.user ? `${host.user}@` : ''}${host.hostName}`))
  return args.join(' ')
}

function jumpEndpoint(host: domain.Host) {
  return `${host.user ? `${host.user}@` : ''}${host.hostName}${host.port && host.port !== 22 ? `:${host.port}` : ''}`
}
function shellQuote(value: string) {
  return `'${value.replace(/'/g, `'\\''`)}'`
}
