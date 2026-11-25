import { useHoverDetailsState } from './hover-details-state'
export { HoverDetailsGroup } from './hover-details-state'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import {
  cloneElement,
  forwardRef,
  useRef,
  type ReactElement,
  type HTMLAttributes,
  type ComponentProps,
  type ReactNode,
  type KeyboardEvent
} from 'react'
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator
} from './context-menu'
import { HoverCard, HoverCardTrigger, HoverCardContent } from './hover-card'
import { ToggleGroup, ToggleGroupItem } from './toggle-group'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from './select'
import { Alert, AlertTitle, AlertDescription } from './alert'
import { Badge } from './badge'
import { Progress } from './progress'
import { Spinner } from './spinner'
import { InputGroup, InputGroupInput, InputGroupAddon, InputGroupButton } from './input-group'
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationPrevious,
  PaginationNext
} from './pagination'
import { AlertCircle, X } from 'lucide-react'
import { Button } from './button'
import { Input } from './input'
import { cycleDialogFocus, Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from './dialog'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator
} from './dropdown-menu'
import { cn } from '@/lib/utils'

export const ActionButton = forwardRef<
  HTMLButtonElement,
  ComponentProps<typeof Button> & { icon?: ReactNode; busy?: boolean }
>(function ActionButton({ icon, busy, children, ...props }, ref) {
  return (
    <Button
      variant="outline"
      {...props}
      ref={ref}
      disabled={props.disabled || busy}
      aria-busy={busy}
      size={props.size ?? (children ? 'sm' : 'icon-sm')}
    >
      {busy ? <Spinner aria-hidden /> : icon}
      {children}
    </Button>
  )
})
export const TextField = forwardRef<
  HTMLInputElement,
  ComponentProps<typeof Input> & {
    leading?: ReactNode
    trailing?: ReactNode
    clearable?: boolean
    onEnter?: (event: KeyboardEvent<HTMLInputElement>) => void
  }
>(function TextField({ leading, trailing, clearable, onEnter, className, ...props }, ref) {
  useTranslation()
  return (
    <InputGroup
      className={cn(
        'text-field [.files-pathbar_>_&]:flex-1 [.files-pathbar_>_&]:min-w-0 [.files-pathbar_>_&]:w-0 [.files-pathbar_>_&]:my-0 [.files-pathbar_>_&]:mx-[5px] [.files-pathbar_>_&]:font-mono [.files-pathbar_>_&]:text-xs [.files-transfer-filters_>_&]:flex-1 [.files-transfer-filters_>_&]:min-w-32.5 [.files-transfer-filters_>_&]:w-45',
        className
      )}
    >
      <InputGroupInput
        {...props}
        ref={ref}
        onKeyDown={(event) => {
          props.onKeyDown?.(event)
          if (
            !event.defaultPrevented &&
            event.key === 'Enter' &&
            !event.nativeEvent.isComposing &&
            event.nativeEvent.keyCode !== 229
          )
            onEnter?.(event)
        }}
      />
      {leading && <InputGroupAddon>{leading}</InputGroupAddon>}
      {(trailing || (clearable && !!props.value)) && (
        <InputGroupAddon align="inline-end">
          {trailing}
          {clearable && !!props.value && (
            <InputGroupButton
              aria-label={t('common:clear')}
              size="icon-xs"
              onClick={(event) => {
                const input = event.currentTarget
                  .closest('[data-slot="input-group"]')
                  ?.querySelector('input')
                if (!input) return
                const setter = Object.getOwnPropertyDescriptor(
                  HTMLInputElement.prototype,
                  'value'
                )?.set
                setter?.call(input, '')
                input.dispatchEvent(new Event('input', { bubbles: true }))
                input.focus()
              }}
            >
              <X />
            </InputGroupButton>
          )}
        </InputGroupAddon>
      )}
    </InputGroup>
  )
})
export type MenuAction = {
  key?: string
  type?: 'divider'
  label?: ReactNode
  icon?: ReactNode
  disabled?: boolean
  danger?: boolean
  onClick?: () => void
}
export function ActionMenu({
  actions,
  children,
  context = false,
  onAction,
  onOpenChange
}: {
  actions: MenuAction[]
  children: ReactNode
  context?: boolean
  onAction?: (event: { key: string }) => void
  onOpenChange?: (open: boolean) => void
}) {
  const selected = useRef(false)
  const previousFocus = useRef<HTMLElement | null>(null)
  function restoreMenuFocus(event: Event) {
    if (!selected.current) return
    event.preventDefault()
    requestAnimationFrame(() => {
      const active = document.activeElement
      if (
        (!active || active === document.body) &&
        !document.querySelector('[data-slot="dialog-content"][data-state="open"]')
      )
        previousFocus.current?.focus({ preventScroll: true })
    })
  }
  const invoke = (item: MenuAction) => {
    selected.current = true
    item.onClick?.()
    if (item.key) onAction?.({ key: item.key })
  }
  const openChange = (open: boolean) => {
    if (open) {
      selected.current = false
      previousFocus.current =
        document.activeElement instanceof HTMLElement ? document.activeElement : null
    }
    onOpenChange?.(open)
  }
  if (context)
    return (
      <ContextMenu onOpenChange={openChange}>
        <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>

        <ContextMenuContent onCloseAutoFocus={restoreMenuFocus}>
          {actions.map((item, index) =>
            item.type === 'divider' ? (
              <ContextMenuSeparator key={index} />
            ) : (
              <ContextMenuItem
                key={item.key}
                variant={item.danger ? 'destructive' : 'default'}
                disabled={item.disabled}
                onSelect={() => invoke(item)}
              >
                {item.icon}
                {item.label}
              </ContextMenuItem>
            )
          )}
        </ContextMenuContent>
      </ContextMenu>
    )
  return (
    <DropdownMenu onOpenChange={openChange}>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent align="end" onCloseAutoFocus={restoreMenuFocus}>
        {actions.map((item, index) =>
          item.type === 'divider' ? (
            <DropdownMenuSeparator key={index} />
          ) : (
            <DropdownMenuItem
              key={item.key}
              disabled={item.disabled}
              variant={item.danger ? 'destructive' : 'default'}
              onSelect={() => invoke(item)}
            >
              {item.icon}
              {item.label}
            </DropdownMenuItem>
          )
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
export const HoverDetails = forwardRef<
  HTMLDivElement,
  Omit<HTMLAttributes<HTMLDivElement>, 'content' | 'children'> & {
    children: ReactElement
    content: ReactNode
    disabled?: boolean
  }
>(function HoverDetails({ children, content, disabled, ...props }, ref) {
  const { open, setOpen, warm, grouped } = useHoverDetailsState(disabled)
  return (
    <HoverCard openDelay={warm ? 0 : 600} open={open} onOpenChange={setOpen}>
      <HoverCardTrigger asChild>{cloneElement(children, { ...props, ref })}</HoverCardTrigger>

      <HoverCardContent
        side="bottom"
        align="start"
        sideOffset={6}
        className={cn(
          'w-auto',
          grouped && 'workspace-tab-hover-details data-[state=closed]:animate-none!'
        )}
      >
        {content}
      </HoverCardContent>
    </HoverCard>
  )
})
export function ChoiceGroup({
  options,
  value,
  onValueChange,
  disabled,
  'aria-label': label
}: {
  options: { label: ReactNode; value: string }[]
  value: string
  onValueChange: (value: string) => void
  disabled?: boolean
  'aria-label'?: string
}) {
  return (
    <ToggleGroup
      className="choice-group [.settings-row-control_&]:w-fit"
      type="single"
      size="sm"
      variant="outline"
      value={value}
      onValueChange={(next) => {
        if (next) onValueChange(next)
      }}
      disabled={disabled}
      aria-label={label}
    >
      {options.map((option) => (
        <ToggleGroupItem key={option.value} value={option.value}>
          {option.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}
export function SelectField({
  options,
  value,
  onValueChange,
  disabled,
  className,
  'aria-label': label,
  'data-testid': testID
}: {
  options: { label: string; value: string }[]
  value: string
  onValueChange: (value: string) => void
  disabled?: boolean
  className?: string
  'aria-label'?: string
  'data-testid'?: string
}) {
  return (
    <Select value={value} onValueChange={onValueChange} disabled={disabled}>
      <SelectTrigger size="sm" data-testid={testID} aria-label={label} className={className}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent position="popper">
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
export function Notice({
  title,
  description,
  tone = 'info',
  action,
  onDismiss,
  className
}: {
  title?: ReactNode
  description?: ReactNode
  tone?: string
  action?: ReactNode
  onDismiss?: () => void
  className?: string
}) {
  useTranslation()
  return (
    <Alert variant={tone === 'error' ? 'destructive' : 'default'} className={className}>
      <AlertCircle />
      {title && <AlertTitle>{title}</AlertTitle>}
      {(description || action || onDismiss) && (
        <AlertDescription>
          {description}
          {action}
          {onDismiss && (
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t('common:dismissNotice')}
              onClick={onDismiss}
            >
              <X />
            </Button>
          )}
        </AlertDescription>
      )}
    </Alert>
  )
}
export function BusyIndicator() {
  useTranslation()
  return <Spinner aria-label={t('common:loading')} />
}
export function StatusBadge({ children, tone }: { children: ReactNode; tone?: string }) {
  return <Badge variant={tone === 'error' ? 'destructive' : 'secondary'}>{children}</Badge>
}
export function WorkbenchDialog({
  open,
  title,
  children,
  onClose,
  onSubmit,
  submitLabel = 'OK',
  submitDisabled,
  busy,
  width = 480,
  actions,
  restoreFocus = true,
  onOpened
}: {
  open: boolean
  title?: ReactNode
  children: ReactNode
  onClose: () => void
  onSubmit?: () => void
  submitLabel?: string
  submitDisabled?: boolean
  busy?: boolean
  width?: number
  actions?: ReactNode
  restoreFocus?: boolean
  onOpened?: (event: Event) => void
}) {
  useTranslation()
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value && !busy) onClose()
      }}
    >
      <DialogContent
        style={{ width, maxWidth: 'calc(100vw - 32px)' }}
        className="flex max-h-[85vh] flex-col gap-4"
        aria-describedby={undefined}
        showCloseButton={!busy}
        onPointerDownOutside={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault()
        }}
        onKeyDownCapture={cycleDialogFocus}
        onKeyDown={(event) => {
          if (
            onSubmit &&
            !busy &&
            !submitDisabled &&
            event.key === 'Enter' &&
            !event.nativeEvent.isComposing &&
            !event.repeat &&
            event.target instanceof HTMLInputElement
          ) {
            event.preventDefault()
            onSubmit()
          }
        }}
        onCloseAutoFocus={(event) => {
          if (!restoreFocus) event.preventDefault()
        }}
        onOpenAutoFocus={onOpened}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div
          className={cn(
            'mac-scroll-area [scrollbar-color:var(--mac-scrollbar-thumb)_var(--mac-scrollbar-track)] [scrollbar-width:thin] [&::-webkit-scrollbar]:w-2.5',
            '[&::-webkit-scrollbar]:h-2.5 [&::-webkit-scrollbar-track]:bg-[var(--mac-scrollbar-track)] [&::-webkit-scrollbar-thumb]:min-h-7',
            '[&::-webkit-scrollbar-thumb]:bg-[var(--mac-scrollbar-thumb)] [&::-webkit-scrollbar-thumb]:[background-clip:padding-box]',
            '[&::-webkit-scrollbar-thumb]:[border:3px_solid_transparent] [&::-webkit-scrollbar-thumb]:rounded-full',
            '[&::-webkit-scrollbar-thumb:hover]:bg-[var(--mac-scrollbar-thumb-hover)] [&::-webkit-scrollbar-corner]:bg-transparent min-h-0 overflow-auto',
            'space-y-3 p-1'
          )}
        >
          {children}
        </div>
        <DialogFooter className="[&>button:focus-visible]:outline-2 [&>button:focus-visible]:outline-offset-2 [&>button:focus-visible]:outline-ring">
          {actions ?? (
            <>
              <Button variant="outline" disabled={busy} onClick={onClose}>
                {t('common:cancel')}
              </Button>
              <ActionButton
                variant="default"
                size="default"
                busy={busy}
                disabled={submitDisabled}
                onClick={onSubmit}
              >
                {submitLabel}
              </ActionButton>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
export function TransferProgress({ value, state }: { value: number; state: string }) {
  useTranslation()
  return (
    <Progress
      value={value}
      aria-label={t('common:transferProgress')}
      data-state={state}
      className={cn(
        state === 'success' &&
          '[&_[data-slot=progress-indicator]]:bg-[var(--files-success,#187044)]',
        state === 'exception' &&
          '[&_[data-slot=progress-indicator]]:bg-[var(--files-error,#b42332)]'
      )}
    />
  )
}
export function TransferPagination({
  page,
  pageSize,
  total,
  onPageChange
}: {
  page: number
  pageSize: number
  total: number
  onPageChange: (page: number, size: number) => void
}) {
  useTranslation()
  const pages = Math.max(1, Math.ceil(total / pageSize))
  return (
    <div className="flex items-center gap-2 text-sm">
      <span className="whitespace-nowrap">{t('common:transferCount', { count: total })}</span>
      <Pagination aria-label={t('common:transferPages')} className="w-auto">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              aria-label={t('common:previousPage')}
              aria-disabled={page <= 1}
              className={page <= 1 ? 'pointer-events-none opacity-50' : undefined}
              onClick={(event) => {
                event.preventDefault()
                if (page > 1) onPageChange(page - 1, pageSize)
              }}
            />
          </PaginationItem>
          <PaginationItem>
            <PaginationLink isActive aria-label={t('common:pagination', { page, pages })} size="sm">
              {page} / {pages}
            </PaginationLink>
          </PaginationItem>
          <PaginationItem>
            <PaginationNext
              href="#"
              aria-label={t('common:nextPage')}
              aria-disabled={page >= pages}
              className={page >= pages ? 'pointer-events-none opacity-50' : undefined}
              onClick={(event) => {
                event.preventDefault()
                if (page < pages) onPageChange(page + 1, pageSize)
              }}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
      <SelectField
        aria-label={t('common:transfersPerPage')}
        value={String(pageSize)}
        options={[25, 50, 100].map((size) => ({
          value: String(size),
          label: t('common:perPage', { size })
        }))}
        onValueChange={(value) => onPageChange(1, Number(value))}
      />
    </div>
  )
}
