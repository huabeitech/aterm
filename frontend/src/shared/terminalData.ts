// Shared limits keep xterm's grid and the underlying PTY in agreement.
export const terminalLimits = { minCols: 2, maxCols: 500, minRows: 1, maxRows: 300 } as const
export const terminalOutputLimit = 8 * 1024 * 1024

export function terminalSize(cols: number, rows: number) {
  return {
    cols: Math.min(terminalLimits.maxCols, Math.max(terminalLimits.minCols, Math.floor(cols))),
    rows: Math.min(terminalLimits.maxRows, Math.max(terminalLimits.minRows, Math.floor(rows)))
  }
}

// Each chunk may be UTF-8 encoded independently by SSH/node-pty. Never split
// a surrogate pair between messages, including at the input paste boundary.
export function* terminalTextChunks(text: string, limit = 16384): Generator<string> {
  if (!Number.isInteger(limit) || limit < 2) throw Error('Invalid terminal chunk size')
  for (let offset = 0; offset < text.length;) {
    let end = Math.min(offset + limit, text.length)
    const last = text.charCodeAt(end - 1)
    const next = text.charCodeAt(end)
    if (last >= 0xd800 && last <= 0xdbff && next >= 0xdc00 && next <= 0xdfff) end--
    yield text.slice(offset, end)
    offset = end
  }
}

export function terminalBinaryInput(value: unknown): Uint8Array {
  if (!(value instanceof Uint8Array) || value.byteLength > 65536)
    throw Error('Invalid terminal binary input')
  return value
}
