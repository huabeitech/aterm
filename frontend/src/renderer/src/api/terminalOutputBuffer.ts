import type {TerminalEvent} from '../../../shared/contracts/terminal'

type Output = {sessionID: string; data?: string; offset: number}
type Stream = {decoder: TextDecoder; offset: number; attached: boolean; attaching: number; pending: Map<number, Uint8Array>}
const bytes = (value: string) => Uint8Array.from(atob(value), character => character.charCodeAt(0))

/** Reconciles Wails byte watermarks before emitting the shared terminal contract.
 * Native event subscription stays in wailsTerminalOutput; this state is scoped to
 * one renderer and can be tested without loading the native runtime. */
export function createTerminalOutputBuffer(emit: (event: TerminalEvent) => void) {
  const streams = new Map<string, Stream>()
  function stream(id: string) {
    let value = streams.get(id)
    if (!value) {
      value = {decoder: new TextDecoder(), offset: 0, attached: false, attaching: 0, pending: new Map()}
      streams.set(id, value)
    }
    return value
  }
  function drain(id: string, value: Stream) {
    if (!value.attached) return
    for (const [end, data] of [...value.pending].sort(([a], [b]) => a - b)) {
      if (end <= value.offset) {value.pending.delete(end); continue}
      const start = end - data.length
      if (start > value.offset) break
      const text = value.decoder.decode(data.subarray(Math.max(0, value.offset - start)), {stream: true})
      value.offset = end
      value.pending.delete(end)
      emit({type: 'data', id, seq: end, data: text})
    }
  }
  return {
    output(output: Output) {
      if (!output.data) return
      // Output is admitted only for an explicitly requested attachment. The
      // backend owns replay before attachment; late events after close must
      // not recreate renderer state for a dead session.
      const value = streams.get(output.sessionID)
      if (!value) return
      value.pending.set(output.offset, bytes(output.data))
      drain(output.sessionID, value)
    },
    beginAttach(id: string) {
      const value = stream(id)
      value.attaching++
      let settled = false
      return {
        complete(snapshot: {data: string; seq: number}) {
          if (settled || streams.get(id) !== value) throw new Error('Terminal attachment was canceled.')
          const decoder = new TextDecoder()
          const data = decoder.decode(bytes(snapshot.data), {stream: true})
          settled = true
          value.attaching--
          value.decoder = decoder
          value.offset = snapshot.seq
          value.attached = true
          drain(id, value)
          return {...snapshot, data}
        },
        cancel() {
          if (settled) return
          settled = true
          value.attaching--
          if (!value.attached && !value.attaching && streams.get(id) === value) streams.delete(id)
        }
      }
    },
    release(id: string) {streams.delete(id)}
  }
}
