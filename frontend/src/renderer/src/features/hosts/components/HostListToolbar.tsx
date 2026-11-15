import { t } from '../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ActionItems, type ActionItem } from '@/components/ui/desktop'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { ChevronDown, Plus, Search, X } from 'lucide-react'

type Props = {
  searchPlaceholder?: string
  hostListQuery: string
  onHostListQueryChange: (query: string) => void
  addDisabled: boolean
  addMenuItems: ActionItem[]
  onAddHost: () => void
  onAddMenuClick: (event: { key: string }) => void
}
export function HostListSearch({
  hostListQuery,
  searchPlaceholder = t('hosts:searchHosts'),
  onHostListQueryChange
}: Pick<Props, 'hostListQuery' | 'onHostListQueryChange' | 'searchPlaceholder'>) {
  useTranslation()
  const [hasFocus, setHasFocus] = useState(false)
  const expanded = hasFocus || hostListQuery.length > 0

  return (
    <div
      className="host-list-search relative min-w-0 max-w-full shrink-0 transition-[width] duration-200 ease-out motion-reduce:transition-none"
      style={{ width: expanded ? 'min(380px, 100%)' : 'calc(11rem * var(--app-layout-scale))' }}
    >
      <Search
        size={14}
        className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        className="pl-8 pr-8 transition-[border-color,box-shadow] duration-200 focus-visible:!ring-2 focus-visible:!ring-ring/40"
        value={hostListQuery}
        onChange={(event) => onHostListQueryChange(event.target.value)}
        onFocus={() => setHasFocus(true)}
        onBlur={() => setHasFocus(false)}
        placeholder={searchPlaceholder}
        title={searchPlaceholder}
        data-action="search-hosts"
        aria-label={t('hosts:searchHosts')}
        data-allow-global-shortcuts="true"
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
      />
      {hostListQuery && (
        <Button
          variant="ghost"
          size="icon"
          className="absolute right-0.5 top-1/2 size-6 -translate-y-1/2"
          aria-label={t('hosts:clearSearch')}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onHostListQueryChange('')}
        >
          <X size={12} />
        </Button>
      )}
    </div>
  )
}
export function HostListAddAction({
  addDisabled,
  addMenuItems,
  onAddHost,
  onAddMenuClick
}: Omit<Props, 'hostListQuery' | 'onHostListQueryChange'>) {
  useTranslation()
  return (
    <div className="flex shrink-0">
      <Button className="rounded-r-none px-3" disabled={addDisabled} onClick={onAddHost}>
        <Plus size={15} />
        {t('hosts:addHost')}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            className="w-7 rounded-l-none border-l border-white/20 px-0"
            disabled={addDisabled}
            aria-label={t('hosts:moreAddOptions')}
          >
            <ChevronDown size={14} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <ActionItems items={addMenuItems} onAction={onAddMenuClick} />
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
