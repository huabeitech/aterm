import type { TerminalSession as Session } from '../../../../shared/contracts/terminal'

// Tab and detail components need session metadata, without PTY geometry.
export type TerminalSession = Omit<Session, 'cols' | 'rows'>
