import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { forwardRef, useId, useState, type ComponentProps, type ReactNode } from 'react'
import { Check, ChevronsUpDown, Eye, EyeOff } from 'lucide-react'
import { Button } from './button'
import { Input } from './input'
import { Field as FormField, FieldLabel, FieldDescription, FieldError } from './field'
import { InputGroup, InputGroupInput, InputGroupAddon, InputGroupButton } from './input-group'
import { Spinner } from './spinner'
import { Tooltip, TooltipContent, TooltipTrigger } from './tooltip'
import { Popover, PopoverContent, PopoverTrigger } from './popover'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList
} from './command'
import { DropdownMenuItem, DropdownMenuSeparator } from './dropdown-menu'
import { cn } from '@/lib/utils'

export function Tip({ title, children }: { title: ReactNode; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent>{title}</TooltipContent>
    </Tooltip>
  )
}
export const IconButton = forwardRef<
  HTMLButtonElement,
  ComponentProps<typeof Button> & { icon?: ReactNode; loading?: boolean }
>(function IconButton({ icon, loading, children, className, ...props }, ref) {
  return (
    <Button
      ref={ref}
      variant="ghost"
      size="icon-sm"
      {...props}
      disabled={props.disabled || loading}
      className={className}
    >
      {loading ? <Spinner /> : icon}
      {children}
    </Button>
  )
})
export function Field({
  label,
  children,
  help,
  extra,
  tooltip,
  required,
  className
}: {
  label?: string
  children: ReactNode
  help?: string
  extra?: string
  tooltip?: string
  required?: boolean
  className?: string
}) {
  const id = useId()
  return (
    <FormField
      data-invalid={!!help}
      className={cn(
        'host-field [.host-editor_&]:mb-0 [.host-editor-reliability-grid_&_>_label]:min-h-9 [.host-editor-reliability-grid_&_>_label]:items-end max-[560px]:[.host-editor-reliability-grid_&_>_label]:min-h-0',
        className
      )}
      role="group"
      aria-labelledby={label ? id : undefined}
    >
      {label && (
        <FieldLabel id={id}>
          {label}
          {required && <span className="text-destructive">*</span>}
          {tooltip && (
            <Tip title={tooltip}>
              <span
                tabIndex={0}
                aria-label={t('common:fieldHelp', { label })}
                className="cursor-help text-muted-foreground"
              >
                ⓘ
              </span>
            </Tip>
          )}
        </FieldLabel>
      )}
      {children}
      {help && <FieldError>{help}</FieldError>}
      {extra && <FieldDescription>{extra}</FieldDescription>}
    </FormField>
  )
}
export const PasswordInput = forwardRef<HTMLInputElement, ComponentProps<typeof Input>>(
  function PasswordInput({ className, ...props }, ref) {
    useTranslation()
    const [visible, setVisible] = useState(false)
    return (
      <InputGroup>
        <InputGroupInput
          {...props}
          ref={ref}
          type={visible ? 'text' : 'password'}
          className={className}
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            disabled={props.disabled}
            size="icon-xs"
            aria-label={visible ? t('common:hidePassword') : t('common:showPassword')}
            onClick={() => setVisible(!visible)}
          >
            {visible ? <EyeOff /> : <Eye />}
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    )
  }
)
export function SearchSelect({
  value,
  onChange,
  options,
  placeholder = t('common:select'),
  clearable,
  label
}: {
  value?: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  placeholder?: string
  clearable?: boolean
  label?: string
}) {
  useTranslation()
  const [open, setOpen] = useState(false)
  const selected = options.find((option) => option.value === value)
  const choices = clearable ? [{ value: '', label: placeholder }, ...options] : options
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-label={label}
          aria-expanded={open}
          className="w-full justify-between font-normal"
        >
          <span className="truncate">{selected?.label ?? placeholder}</span>
          <ChevronsUpDown className="size-3.5 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        aria-label={label}
        className="w-[var(--radix-popover-trigger-width)] p-0"
        align="start"
      >
        <Command label={t('common:searchOptions', { label: label ?? '' })}>
          <CommandInput placeholder={t('common:search')} />
          <CommandList>
            <CommandEmpty>{t('common:noResults')}</CommandEmpty>
            <CommandGroup>
              {choices.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value || '__empty'}
                  keywords={[option.label]}
                  onSelect={() => {
                    onChange(option.value)
                    setOpen(false)
                  }}
                >
                  <Check className={cn('size-3.5', value !== option.value && 'invisible')} />
                  {option.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
export type ActionItem = {
  type?: 'divider'
  key?: string
  label?: ReactNode
  icon?: ReactNode
  danger?: boolean
  disabled?: boolean
}
export function ActionItems({
  items,
  onAction
}: {
  items: ActionItem[]
  onAction: (event: { key: string }) => void
}) {
  return (
    <>
      {items.map((item, index) =>
        item.type === 'divider' ? (
          <DropdownMenuSeparator key={`separator-${index}`} />
        ) : (
          <DropdownMenuItem
            key={item.key}
            disabled={item.disabled}
            variant={item.danger ? 'destructive' : 'default'}
            onSelect={() => onAction({ key: item.key! })}
          >
            {item.icon}
            {item.label}
          </DropdownMenuItem>
        )
      )}
    </>
  )
}
