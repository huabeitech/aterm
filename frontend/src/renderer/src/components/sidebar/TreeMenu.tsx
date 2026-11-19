import { useRef, type ReactNode } from 'react'
import { ContextMenu, DropdownMenu } from 'radix-ui'
import { cn } from '@/lib/utils'

export const menuClass =
  'z-[1200] max-h-96 min-w-48 overflow-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md'
export const itemClass =
  'flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none focus:bg-accent data-[disabled]:pointer-events-none data-[disabled]:opacity-50'
export type Action = {
  label: string
  icon?: ReactNode
  run: () => void
  danger?: boolean
  disabled?: boolean
  afterClose?: boolean
}

export function TreeMenu({
  children,
  actions,
  disabled,
  dropdown = false
}: {
  children: ReactNode
  actions: Action[]
  disabled: boolean
  dropdown?: boolean
}) {
  const selectedAction = useRef<Action | null>(null)
  const Menu = dropdown ? DropdownMenu : ContextMenu
  const renderAction = (action: Action) => (
    <Menu.Item
      key={action.label}
      disabled={disabled || action.disabled}
      className={cn(itemClass, action.danger && 'text-destructive focus:text-destructive')}
      onSelect={() => {
        selectedAction.current = action
        if (!action.afterClose) action.run()
      }}
    >
      {action.icon}
      {action.label}
    </Menu.Item>
  )
  // Locking page scroll when a context menu opens can shift the sidebar.
  return (
    <Menu.Root
      modal={dropdown}
      onOpenChange={(open) => {
        if (open) selectedAction.current = null
      }}
    >
      <Menu.Trigger asChild>{children}</Menu.Trigger>
      <Menu.Portal>
        <Menu.Content
          className={menuClass}
          onCloseAutoFocus={(event) => {
            const action = selectedAction.current
            selectedAction.current = null
            if (action) {
              event.preventDefault()
              // Focus-moving actions must wait for the menu focus trap to close.
              if (action.afterClose) action.run()
            }
          }}
        >
          {actions.filter((action) => !action.danger).map(renderAction)}
          {actions.some((action) => action.danger) && (
            <Menu.Separator className="my-1 h-px bg-border" />
          )}
          {actions.filter((action) => action.danger).map(renderAction)}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  )
}
