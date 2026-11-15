/** Includes the selected directory and every descendant, regardless of tree order. */
export function directoryScopeIDs(
  directories: { id: string; parentID: string }[],
  directoryID: string
): Set<string> {
  const children = new Map<string, string[]>()
  for (const directory of directories) {
    const siblings = children.get(directory.parentID) ?? []
    siblings.push(directory.id)
    children.set(directory.parentID, siblings)
  }
  const result = new Set<string>()
  const pending = [directoryID]
  while (pending.length) {
    const id = pending.pop()!
    if (result.has(id)) continue
    result.add(id)
    pending.push(...(children.get(id) ?? []))
  }
  return result
}
