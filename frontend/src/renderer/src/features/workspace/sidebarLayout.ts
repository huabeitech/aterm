export const sidebarLayout = {
  defaultWidth: 260,
  minWidth: 180,
  maxWidth: 560,
  minimumContentWidth: 580
} as const

export function constrainSidebarWidth(width: number): number {
  return Math.max(sidebarLayout.minWidth, Math.min(sidebarLayout.maxWidth, width))
}
