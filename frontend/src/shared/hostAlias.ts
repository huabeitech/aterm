export const HOST_ALIAS_FORMAT_ERROR =
  'Host Alias can only contain letters, numbers, dots, underscores, or hyphens.'

export function isValidHostAlias(alias: string): boolean {
  return /^[a-zA-Z0-9_.-]+$/.test(alias)
}
