import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from './button'
import { cycleDialogFocus, Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from './dialog'

export type ConfirmationContent = {
  title: ReactNode
  content?: ReactNode
  okText?: string
  cancelText?: string
  className?: string
  overlayClassName?: string
  icon?: ReactNode
  descriptionId?: string
  submitOnEnter?: boolean
  afterOpenChange?: (open: boolean) => void
}

type ConfirmationDialogProps = ConfirmationContent & {
  pending?: boolean
  confirmDisabled?: boolean
  danger?: boolean
  extraAction?: { label: string; onClick: () => void }
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmationDialog({
  title,
  content,
  okText = 'OK',
  cancelText,
  className,
  overlayClassName,
  icon,
  descriptionId,
  submitOnEnter,
  afterOpenChange,
  pending,
  confirmDisabled,
  danger,
  extraAction,
  onConfirm,
  onCancel
}: ConfirmationDialogProps) {
  const { t } = useTranslation('common')
  const contentId = useId()
  const blocked = pending || confirmDisabled

  return (
    <Dialog open onOpenChange={(open) => !open && !pending && onCancel()}>
      <DialogContent
        className={cn('gap-4 sm:max-w-[440px]', className)}
        overlayClassName={overlayClassName}
        showCloseButton={false}
        aria-describedby={descriptionId ?? (content ? contentId : undefined)}
        onPointerDownOutside={(event) => event.preventDefault()}
        onOpenAutoFocus={() => afterOpenChange?.(true)}
        onEscapeKeyDown={(event) => {
          if (pending) event.preventDefault()
        }}
        onKeyDownCapture={cycleDialogFocus}
        onKeyDown={(event) => {
          if (
            submitOnEnter &&
            !event.defaultPrevented &&
            !event.nativeEvent.isComposing &&
            event.key === 'Enter' &&
            !event.repeat &&
            event.target instanceof HTMLInputElement
          ) {
            event.preventDefault()
            if (!blocked) onConfirm()
          }
        }}
      >
        <DialogHeader>
          {icon}
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {content && (
          <div id={contentId} className="min-w-0 text-[length:var(--app-text-13)] leading-relaxed">
            {content}
          </div>
        )}
        <DialogFooter className="flex-col flex-wrap [&>button:focus-visible]:outline-2 [&>button:focus-visible]:outline-offset-2 [&>button:focus-visible]:outline-ring">
          <Button variant="outline" disabled={pending} onClick={onCancel}>
            {cancelText ?? t('cancel')}
          </Button>
          {extraAction && (
            <Button variant="outline" disabled={pending} onClick={extraAction.onClick}>
              {extraAction.label}
            </Button>
          )}
          <Button
            variant={danger ? 'destructive' : 'default'}
            disabled={blocked}
            onClick={onConfirm}
          >
            {pending && <Loader2 className="size-3.5 animate-spin" />}
            {okText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
