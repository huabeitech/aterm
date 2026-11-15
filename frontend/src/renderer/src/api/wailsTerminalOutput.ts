import {Events} from '@wailsio/runtime'
import type {TerminalEvent} from '../../../shared/contracts/terminal'
import {createTerminalOutputBuffer} from './terminalOutputBuffer'

const listeners = new Set<(event: TerminalEvent) => void>()
const buffer = createTerminalOutputBuffer(event => {
  for (const listener of listeners) listener(event)
})
if (location.hash !== '#file-editor') Events.On('terminal:output', event => buffer.output(event.data))
export function subscribeTerminalOutput(callback: (event: TerminalEvent) => void) {
  listeners.add(callback)
  const off = Events.On('terminal:event', event => callback(event.data as TerminalEvent))
  return () => {listeners.delete(callback); off()}
}
export const beginTerminalOutputAttachment = buffer.beginAttach
export const releaseTerminalOutput = buffer.release
