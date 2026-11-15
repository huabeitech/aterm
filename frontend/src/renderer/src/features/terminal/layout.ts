export type Axis = 'vertical' | 'horizontal'
export type PaneNode =
  | { kind: 'leaf'; id: string }
  | { kind: 'split'; id: string; axis: Axis; ratio: number; first: PaneNode; second: PaneNode }
export const leaf = (id: string): PaneNode => ({ kind: 'leaf', id })
export function paneIDs(node: PaneNode): string[] {
  return node.kind === 'leaf' ? [node.id] : [...paneIDs(node.first), ...paneIDs(node.second)]
}
export function splitPane(node: PaneNode, id: string, nextID: string, axis: Axis): PaneNode {
  if (node.kind === 'leaf')
    return node.id === id
      ? { kind: 'split', id: nextID, axis, ratio: 0.5, first: node, second: leaf(nextID) }
      : node
  return {
    ...node,
    first: splitPane(node.first, id, nextID, axis),
    second: splitPane(node.second, id, nextID, axis)
  }
}
export function removePane(node: PaneNode, id: string): PaneNode | null {
  if (node.kind === 'leaf') return node.id === id ? null : node
  const first = removePane(node.first, id),
    second = removePane(node.second, id)
  return first && second ? { ...node, first, second } : (first ?? second)
}
export function resizeSplit(node: PaneNode, id: string, ratio: number): PaneNode {
  if (node.kind === 'leaf') return node
  if (node.id === id) return { ...node, ratio: Math.min(0.85, Math.max(0.15, ratio)) }
  return {
    ...node,
    first: resizeSplit(node.first, id, ratio),
    second: resizeSplit(node.second, id, ratio)
  }
}
export interface Box {
  x: number
  y: number
  width: number
  height: number
}
export function layoutPanes(
  node: PaneNode,
  box: Box = { x: 0, y: 0, width: 100, height: 100 },
  panes: { id: string; box: Box }[] = [],
  handles: { node: Extract<PaneNode, { kind: 'split' }>; box: Box }[] = []
) {
  if (node.kind === 'leaf') panes.push({ id: node.id, box })
  else {
    handles.push({ node, box })
    const { x, y, width, height } = box
    if (node.axis === 'vertical') {
      layoutPanes(node.first, { x, y, width: width * node.ratio, height }, panes, handles)
      layoutPanes(
        node.second,
        { x: x + width * node.ratio, y, width: width * (1 - node.ratio), height },
        panes,
        handles
      )
    } else {
      layoutPanes(node.first, { x, y, width, height: height * node.ratio }, panes, handles)
      layoutPanes(
        node.second,
        { x, y: y + height * node.ratio, width, height: height * (1 - node.ratio) },
        panes,
        handles
      )
    }
  }
  return { panes, handles }
}
