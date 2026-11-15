export function getVisibleSelectedHost<T extends { id: string }>(
  hosts: T[],
  selectedID: string | null
): T | null {
  if (!selectedID) return null
  return hosts.find((host) => host.id === selectedID) ?? null
}

export type HostSelectionInput<T extends { id: string }> = {
  visibleHosts: T[]
  selectedIDs: string[]
  anchorID: string | null
  clickedID: string
  additive: boolean
  range: boolean
}

export type HostSelectionState = {
  selectedIDs: string[]
  anchorID: string | null
}

export function updateHostSelection<T extends { id: string }>({
  visibleHosts,
  selectedIDs,
  anchorID,
  clickedID,
  additive,
  range
}: HostSelectionInput<T>): HostSelectionState {
  const visibleIDs = visibleHosts.map((host) => host.id)
  if (!visibleIDs.includes(clickedID)) {
    return { selectedIDs: selectedIDs.filter((id) => visibleIDs.includes(id)), anchorID }
  }

  if (range) {
    const rangeAnchor =
      anchorID && visibleIDs.includes(anchorID)
        ? anchorID
        : (selectedIDs.find((id) => visibleIDs.includes(id)) ?? clickedID)
    const anchorIndex = visibleIDs.indexOf(rangeAnchor)
    const clickedIndex = visibleIDs.indexOf(clickedID)
    const [start, end] =
      anchorIndex < clickedIndex ? [anchorIndex, clickedIndex] : [clickedIndex, anchorIndex]
    return { selectedIDs: visibleIDs.slice(start, end + 1), anchorID: rangeAnchor }
  }

  if (additive) {
    const next = selectedIDs.includes(clickedID)
      ? selectedIDs.filter((id) => id !== clickedID)
      : [...selectedIDs.filter((id) => visibleIDs.includes(id)), clickedID]
    return { selectedIDs: next, anchorID: clickedID }
  }

  return { selectedIDs: [clickedID], anchorID: clickedID }
}

export function getHostDragIDs(selectedIDs: string[], draggedID: string): string[] {
  return selectedIDs.includes(draggedID) ? selectedIDs : [draggedID]
}
