import { createContext, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { toast } from './toast'
import { ConfirmationDialog, type ConfirmationContent } from './confirmation-dialog'

export type ConfirmationOptions = ConfirmationContent & {
  okButtonProps?: { danger?: boolean; loading?: boolean; disabled?: boolean }
  onOk?: () => void | Promise<unknown>
  onCancel?: () => void
}
type Entry = { id: number; options: ConfirmationOptions; pending: boolean }
export type ConfirmationService = {
  confirm: (options: ConfirmationOptions) => {
    update: (options: Partial<ConfirmationOptions>) => void
    destroy: () => void
  }
}
const ConfirmationContext = createContext<ConfirmationService | null>(null)
export function ConfirmationProvider({ children }: { children: ReactNode }) {
  const [entries, setEntries] = useState<Entry[]>([])
  const nextID = useRef(0)
  const running = useRef(new Set<number>())
  const service = useMemo<ConfirmationService>(
    () => ({
      confirm(options) {
        const id = ++nextID.current
        setEntries((current) => [...current, { id, options, pending: false }])
        return {
          update: (update) =>
            setEntries((current) =>
              current.map((entry) =>
                entry.id === id ? { ...entry, options: { ...entry.options, ...update } } : entry
              )
            ),
          destroy: () => setEntries((current) => current.filter((entry) => entry.id !== id))
        }
      }
    }),
    []
  )
  function remove(id: number) {
    setEntries((current) => current.filter((entry) => entry.id !== id))
  }
  async function accept(entry: Entry, onOk: ConfirmationOptions['onOk']) {
    if (running.current.has(entry.id) || entry.options.okButtonProps?.loading) return
    running.current.add(entry.id)
    setEntries((current) =>
      current.map((item) => (item.id === entry.id ? { ...item, pending: true } : item))
    )
    try {
      await onOk?.()
      remove(entry.id)
    } catch {
      setEntries((current) =>
        current.map((item) => (item.id === entry.id ? { ...item, pending: false } : item))
      )
    } finally {
      running.current.delete(entry.id)
    }
  }
  return (
    <ConfirmationContext.Provider value={service}>
      {children}
      {entries.map((entry) => {
        const { onOk, onCancel, okButtonProps, ...dialogProps } = entry.options
        const pending = entry.pending || okButtonProps?.loading
        function cancel() {
          if (pending || running.current.has(entry.id)) return
          onCancel?.()
          remove(entry.id)
        }
        return (
          <ConfirmationDialog
            key={entry.id}
            {...dialogProps}
            pending={pending}
            confirmDisabled={okButtonProps?.disabled}
            danger={okButtonProps?.danger}
            onConfirm={() => void accept(entry, onOk)}
            onCancel={cancel}
          />
        )
      })}
    </ConfirmationContext.Provider>
  )
}
export function useAppUI() {
  const modal = useContext(ConfirmationContext)
  if (!modal) throw new Error('ConfirmationProvider is missing')
  return useMemo(
    () => ({
      modal,
      message: {
        warning: (text: string) => {
          toast.add({ title: text, type: 'warning' })
        },
        success: (text: string) => {
          toast.add({ title: text, type: 'success' })
        },
        error: (text: string) => {
          toast.add({ title: text, type: 'error' })
        },
        info: (text: string) => {
          toast.add({ title: text, type: 'info' })
        }
      },
      notification: {
        warning: ({ title, description }: { title: string; description?: string }) =>
          toast.add({ title, description, type: 'warning' }),
        error: ({
          title,
          message,
          description
        }: {
          title?: string
          message?: string
          description?: string
        }) => toast.add({ title: title ?? message, description, type: 'error' })
      }
    }),
    [modal]
  )
}
