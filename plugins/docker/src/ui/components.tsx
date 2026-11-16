import {
  forwardRef,
  useEffect,
  useState,
  type ButtonHTMLAttributes,
  type ReactNode,
  type ComponentProps,
  type Ref
} from 'react'
import { DropdownMenu, Popover, ScrollArea as ScrollAreaPrimitive } from 'radix-ui'
import { Check, ChevronDown, ListFilter, Copy } from 'lucide-react'

export const Button = forwardRef<
  HTMLButtonElement,
  ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: 'ghost' | 'outline' | 'primary' | 'destructive'
    icon?: boolean
  }
>(function Button({ variant = 'ghost', icon = false, className = '', children, ...props }, ref) {
  return (
    <button
      ref={ref}
      type="button"
      data-slot="button"
      className={`button button-sm ${variant} ${icon ? 'icon-button' : ''} ${className}`}
      {...props}
    >
      {children}
    </button>
  )
})
export function MenuItem({
  children,
  destructive = false,
  ...props
}: ComponentProps<typeof DropdownMenu.Item> & { destructive?: boolean }) {
  return (
    <DropdownMenu.Item className={`menu-item body-6 ${destructive ? 'danger' : ''}`} {...props}>
      {children}
    </DropdownMenu.Item>
  )
}
export function Menu({
  trigger,
  label,
  children
}: {
  trigger: ReactNode
  label: string
  children: ReactNode
}) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="menu" sideOffset={8} align="end" aria-label={label}>
          {children}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
export function StatusFilter({
  value,
  onChange,
  options,
  label
}: {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  label: string
}) {
  return (
    <Menu
      label={label}
      trigger={
        <Button variant="outline" className="filter-trigger" aria-label={label}>
          <ListFilter className="filter-glyph" />
          <span>{options.find((o) => o.value === value)?.label}</span>
          <ChevronDown className="filter-chevron" />
        </Button>
      }
    >
      <DropdownMenu.RadioGroup value={value} onValueChange={onChange}>
        {options.map((option) => (
          <DropdownMenu.RadioItem
            className="menu-item body-6 filter-option"
            key={option.value}
            value={option.value}
          >
            <span>{option.label}</span>
            <DropdownMenu.ItemIndicator>
              <Check />
            </DropdownMenu.ItemIndicator>
          </DropdownMenu.RadioItem>
        ))}
      </DropdownMenu.RadioGroup>
    </Menu>
  )
}
export function Section({
  title,
  count,
  children
}: {
  title: string
  count?: number
  children: ReactNode
}) {
  return (
    <section className="detail-section">
      <h3 className="title-5">
        {title}
        {count !== undefined && count > 0 && (
          <span className="section-count caption-5">{count}</span>
        )}
      </h3>
      {children}
    </section>
  )
}
export { DropdownMenu, Popover }

/** Same Radix anatomy, dimensions and theme tokens as the app's ScrollArea. */
export function ScrollArea({
  children,
  className = '',
  viewportRef,
  viewportClassName = '',
  ...props
}: {
  children: ReactNode
  className?: string
  viewportRef?: Ref<HTMLDivElement>
  viewportClassName?: string
} & ComponentProps<typeof ScrollAreaPrimitive.Root>) {
  return (
    <ScrollAreaPrimitive.Root
      data-slot="scroll-area"
      className={`scroll-area ${className}`}
      {...props}
    >
      <ScrollAreaPrimitive.Viewport
        ref={viewportRef}
        data-slot="scroll-area-viewport"
        className={`scroll-viewport ${viewportClassName}`}
      >
        {children}
      </ScrollAreaPrimitive.Viewport>
      {(['vertical', 'horizontal'] as const).map((orientation) => (
        <ScrollAreaPrimitive.Scrollbar
          key={orientation}
          orientation={orientation}
          data-slot="scroll-area-scrollbar"
          className="scroll-bar"
        >
          <ScrollAreaPrimitive.Thumb data-slot="scroll-area-thumb" className="scroll-thumb" />
        </ScrollAreaPrimitive.Scrollbar>
      ))}
      <ScrollAreaPrimitive.Corner />
    </ScrollAreaPrimitive.Root>
  )
}

async function copyText(value: string) {
  // The isolated document has no clipboard bridge; user-initiated copy preserves text selection as fallback.
  const focus = document.activeElement as HTMLElement | null
  const selection = document.getSelection()
  const ranges = selection
    ? Array.from({ length: selection.rangeCount }, (_, index) =>
        selection.getRangeAt(index).cloneRange()
      )
    : []
  const node = document.createElement('textarea')
  node.value = value
  node.className = 'copy-buffer'
  document.body.append(node)
  node.select()
  try {
    return document.execCommand('copy')
  } finally {
    node.remove()
    focus?.focus({ preventScroll: true })
    if (selection) {
      selection.removeAllRanges()
      ranges.forEach((range) => selection.addRange(range))
    }
  }
}
export function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), 1600)
    return () => clearTimeout(timer)
  }, [copied])
  return (
    <Button
      icon
      title={label}
      aria-label={label}
      onClick={() => {
        void copyText(value)
          .then(setCopied)
          .catch(() => {})
      }}
    >
      {copied ? <Check /> : <Copy />}
    </Button>
  )
}

/** Shared initial list placeholder keeps container and image loading states aligned. */
export function ListSkeleton({ label }: { label: string }) {
  return (
    <div className="list-skeleton" aria-label={label} aria-busy="true" role="status">
      {[1, 2, 3, 4, 5].map((row) => (
        <div className="skeleton-row" key={row} aria-hidden="true">
          <div className="skeleton" />
          <div className="skeleton" />
          <div className="skeleton" />
        </div>
      ))}
    </div>
  )
}
