type HostLike = { id: string; directoryID: string }

export function applyOptimisticHostDirectoryMove<Host extends HostLike>(
  hosts: Host[],
  hostIDs: string[],
  directoryID: string
): Host[] {
  const movingIDs = new Set(hostIDs)
  return hosts.map((host) => (movingIDs.has(host.id) ? { ...host, directoryID } : host))
}
