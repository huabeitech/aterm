export type ConfirmationChoice = 'cancel' | 'accept' | 'extra'

export interface WindowConfirmation {
  titleKey?: string
  messageKey?: string
  confirmKey?: string
  cancelKey?: string
  parameters?: Record<string,string>
  title: string
  message: string
  confirmLabel: string
  cancelLabel?: string
  extraLabel?: string
  danger?: boolean
}

export type ConfirmationEvent =
  | { type: 'open'; id: string; options: WindowConfirmation }
  | { type: 'close'; id: string }
