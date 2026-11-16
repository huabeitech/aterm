/** Shared limits for streaming output batches and the visible tail window. */
export const logLimits = { lines: 300, bytes: 24576 } as const
export function logSnapshot(text: string, truncated = false) {
  // Walk backward by code point so clipping cannot split a UTF-8 character.
  let start = text.length
  let bytes = 0
  while (start > 0) {
    let previous = start - 1
    const unit = text.charCodeAt(previous)
    if (unit >= 0xdc00 && unit <= 0xdfff && previous > 0) {
      const lead = text.charCodeAt(previous - 1)
      if (lead >= 0xd800 && lead <= 0xdbff) previous--
    }
    const code = text.codePointAt(previous)!
    const size = code <= 0x7f ? 1 : code <= 0x7ff ? 2 : code <= 0xffff ? 3 : 4
    if (bytes + size > logLimits.bytes) break
    bytes += size
    start = previous
  }
  if (start > 0) {
    text = text.slice(start)
    truncated = true
  }
  const trailingNewline = text.endsWith('\n')
  const lines = text.split('\n')
  if (trailingNewline) lines.pop()
  if (lines.length > logLimits.lines) {
    text = lines.slice(-logLimits.lines).join('\n') + (trailingNewline ? '\n' : '')
    truncated = true
  }
  return { text, truncated }
}

/** Incremental log buffer. A partial final line is retained across stream batches. */
export class LogBuffer {
  private value = { text: '', truncated: false }
  append(text: string, truncated = false) {
    // Producer overflow creates a gap. Never join a prior partial line across it.
    if (truncated) this.value = { text: '', truncated: true }
    this.value = logSnapshot(this.value.text + text, this.value.truncated || truncated)
    return this.value
  }
}
