import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { createContext, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import { toast } from './toast'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from './dialog'
import { Button } from './button'
import { cn } from '@/lib/utils'

export type ConfirmationOptions = {
  title: ReactNode
  content?: ReactNode
  okText?: string
  cancelText?: string
  className?: string
  overlayClassName?: string
  icon?: ReactNode
  descriptionId?: string
  submitOnEnter?: boolean
  okButtonProps?: { danger?: boolean; loading?: boolean; disabled?: boolean }
  onOk?: () => void | Promise<unknown>
  onCancel?: () => void
  afterOpenChange?: (open: boolean) => void
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
  useTranslation()
  const [entries, setEntries] = useState<Entry[]>([])
  const nextID = useRef(0)
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
  async function accept(entry: Entry) {
    if (
      entry.pending ||
      entry.options.okButtonProps?.loading ||
      entry.options.okButtonProps?.disabled
    )
      return
    setEntries((current) =>
      current.map((item) => (item.id === entry.id ? { ...item, pending: true } : item))
    )
    try {
      await entry.options.onOk?.()
      remove(entry.id)
    } catch {
      setEntries((current) =>
        current.map((item) => (item.id === entry.id ? { ...item, pending: false } : item))
      )
    }
  }
  return (
    <ConfirmationContext.Provider value={service}>
      {children}
      {entries.map((entry) => {
        const { options } = entry
        const pending = entry.pending || options.okButtonProps?.loading
        function cancel() {
          if (pending) return
          options.onCancel?.()
          remove(entry.id)
        }
        return (
          <Dialog
            key={entry.id}
            open
            onOpenChange={(open) => {
              if (!open) cancel()
            }}
          >
            <DialogContent
              className={cn('gap-4 sm:max-w-[440px]', options.className)}
              overlayClassName={options.overlayClassName}
              showCloseButton={false}
              aria-describedby={options.descriptionId}
              onPointerDownOutside={(event) => event.preventDefault()}
              onOpenAutoFocus={() => options.afterOpenChange?.(true)}
              onEscapeKeyDown={(event) => {
                if (pending) event.preventDefault()
              }}
              onKeyDown={(event) => {
                if (
                  options.submitOnEnter &&
                  event.key === 'Enter' &&
                  !event.nativeEvent.isComposing &&
                  !event.repeat &&
                  event.target instanceof HTMLInputElement
                ) {
                  event.preventDefault()
                  void accept(entry)
                }
              }}
            >
              <DialogHeader>
                {options.icon}
                <DialogTitle>{options.title}</DialogTitle>
              </DialogHeader>
              <div className="text-[length:var(--app-text-13)] leading-relaxed">
                {options.content}
              </div>
              <DialogFooter>
                <Button variant="outline" disabled={pending} onClick={cancel}>
                  {options.cancelText ?? t('common:cancel')}
                </Button>
                <Button
                  variant={options.okButtonProps?.danger ? 'destructive' : 'default'}
                  disabled={pending || options.okButtonProps?.disabled}
                  onClick={() => void accept(entry)}
                >
                  {pending && <Loader2 className="size-3.5 animate-spin" />}
                  {options.okText ?? 'OK'}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
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
