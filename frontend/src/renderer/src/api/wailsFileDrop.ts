/** Preserve the shared React drop handler while Wails supplies native file paths.
 * No file contents cross IPC. Synthetic File objects carry only the native path.
 */
export function installNativeFileDrop() {
  if (location.hash === '#file-editor') return
  const mark = () => document.querySelectorAll('.files-workspace').forEach((node) => node.setAttribute('data-file-drop-target', 'true'))
  new MutationObserver(mark).observe(document.documentElement, {childList: true, subtree: true})
  mark()
  type Runtime = {handlePlatformFileDrop?: (paths: string[], x: number, y: number) => void}
  const runtime = (window as Window & {_wails?: Runtime})._wails
  if (!runtime?.handlePlatformFileDrop) return
  const original = runtime.handlePlatformFileDrop
  runtime.handlePlatformFileDrop = (paths, x, y) => {
    const target = document.elementFromPoint(x, y)?.closest('.files-workspace')
    if (target && paths.length) {
      const transfer = new DataTransfer()
      for (const path of paths) {
        const file = new File([], path.split(/[\\/]/).pop() ?? 'file')
        Object.defineProperty(file, 'path', {value: path})
        transfer.items.add(file)
      }
      target.dispatchEvent(new DragEvent('drop', {bubbles: true, cancelable: true, clientX: x, clientY: y, dataTransfer: transfer}))
    }
    original(paths, x, y)
  }
  document.addEventListener('drop', (event) => {
    if (!event.target || !(event.target instanceof Element) || !event.target.closest('.files-workspace')) return
    if (!event.dataTransfer?.types.includes('Files')) return
    const files = Array.from(event.dataTransfer.files) as (File & {path?: string})[]
    // Native macOS drops arrive without resolvable DOM paths. Their platform
    // callback above replays the drop exactly once with the authoritative paths.
    if (!files.length || files.some((file) => !file.path)) {event.preventDefault(); event.stopPropagation()}
  }, true)
}
