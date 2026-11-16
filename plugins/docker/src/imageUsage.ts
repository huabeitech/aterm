import type { DockerDeadline } from './deadline'
import type { DockerExecutor } from './operations'
const imageKey = (id: string) => id.replace(/^sha256:/, '')
/** Read immutable image IDs in bounded container batches, including stopped containers.
 * Tags can be reassigned, so joining the human-readable `ps.Image` field is unsafe. */
export async function imageUsage(context: string, execute: DockerExecutor, signal: DockerDeadline) {
  for (let attempt = 0; ; attempt++) {
    try {
      const output = await execute(
        ['--context', context, 'ps', '-a', '--quiet', '--no-trunc'],
        signal
      )
      const ids = [...new Set(output.trim().split('\n').filter(Boolean))]
      if (ids.some((id) => !/^[a-f0-9]{64}$/.test(id)))
        throw Error('Invalid container ID from Docker')
      const counts = new Map<string, number>()
      for (let offset = 0; offset < ids.length; offset += 100) {
        const batch = ids.slice(offset, offset + 100)
        const rows = await execute(
          [
            '--context',
            context,
            'inspect',
            '--type',
            'container',
            '--format',
            '{"ID":{{json .Id}},"ImageID":{{json .Image}}}',
            ...batch
          ],
          signal
        )
        const seen = new Set<string>()
        for (const line of rows.trim().split('\n').filter(Boolean)) {
          const row = JSON.parse(line)
          if (
            !batch.includes(row.ID) ||
            seen.has(row.ID) ||
            typeof row.ImageID !== 'string' ||
            !/^(?:sha256:)?[a-f0-9]{64}$/.test(row.ImageID)
          )
            throw Error('Invalid container image reference from Docker')
          seen.add(row.ID)
          const key = imageKey(row.ImageID)
          counts.set(key, (counts.get(key) ?? 0) + 1)
        }
        if (seen.size !== batch.length)
          throw Error('Incomplete container image references from Docker')
      }
      return (id: string) => counts.get(imageKey(id)) ?? 0
    } catch (error) {
      // A container may disappear between listing and inspection. Retry the snapshot once.
      if (attempt === 0 && /No such (object|container)/i.test(String((error as Error).message)))
        continue
      throw error
    }
  }
}
