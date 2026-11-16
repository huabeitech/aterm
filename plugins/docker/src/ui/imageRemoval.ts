export interface ImageRemovalResult {
  deleted: string[]
  failed: { id: string; message: string }[]
}

/** Keep each deletion independent so Docker can reject one image without
 * hiding the result for the rest. Stop issuing commands when the view leaves
 * the engine that supplied the list. */
export async function removeSelectedImages(
  ids: string[],
  remove: (id: string) => Promise<unknown>,
  isCurrent: () => boolean
): Promise<ImageRemovalResult | undefined> {
  const result: ImageRemovalResult = { deleted: [], failed: [] }
  for (const id of new Set(ids)) {
    if (!isCurrent()) return
    try {
      await remove(id)
      result.deleted.push(id)
    } catch (error) {
      result.failed.push({ id, message: error instanceof Error ? error.message : String(error) })
    }
  }
  if (isCurrent()) return result
}
