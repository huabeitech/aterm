import { useId, useRef, useState, type KeyboardEvent } from 'react'
import { ChevronDown, Clock3, Pin, PinOff, Plus, Search, Terminal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { t } from '../../../../shared/i18n'
import type { domain } from '../../api/types'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { compareHostsInScope } from '../../features/hosts/model/hostModel'
import { formatCommandPaletteShortcut } from '../common/commandPaletteShortcut'

export function ConnectionWelcome({
  hosts,
  searchShortcut,
  onCreate,
  onSearch,
  onConnect,
  onTogglePinned,
  pinBusy,
  onLocalTerminal
}: {
  hosts: domain.Host[]
  searchShortcut: string
  onCreate: () => void
  onSearch: () => void
  onConnect: (id: string) => void
  onTogglePinned: (id: string) => void
  pinBusy: boolean
  onLocalTerminal?: () => void
}) {
  useTranslation()
  const searchRef = useRef<HTMLButtonElement>(null)
  const connectionsRef = useRef<HTMLDivElement>(null)
  const pinnedListID = useId()
  const [showAllPinned, setShowAllPinned] = useState(false)
  const pinned = hosts
    .filter((host) => host.favorite)
    .sort((a, b) => compareHostsInScope(a, b, { type: 'favorites', id: '' }))
  const visiblePinned = showAllPinned ? pinned : pinned.slice(0, 3)
  const recent = hosts
    .filter((host) => !host.favorite && Number.isFinite(Date.parse(host.lastUsedAt)))
    .sort(
      (a, b) =>
        Date.parse(b.lastUsedAt) - Date.parse(a.lastUsedAt) || a.alias.localeCompare(b.alias)
    )
    .slice(0, 9)

  function navigateConnections(event: KeyboardEvent<HTMLDivElement>) {
    const buttons = Array.from(
      connectionsRef.current?.querySelectorAll<HTMLButtonElement>('[data-launchpad-nav]') ?? []
    )
    const active = document.activeElement as HTMLButtonElement
    const connection = active
      .closest('li')
      ?.querySelector<HTMLButtonElement>('[data-launchpad-nav]')
    const index = buttons.indexOf(connection ?? active)
    if (index < 0) return
    let next = index
    const cards = connection
      ? Array.from(
          connection.closest('ul')?.querySelectorAll<HTMLButtonElement>('[data-launchpad-nav]') ??
            []
        )
      : []
    const columnIndex = cards.indexOf(connection!)
    if (event.key === 'ArrowDown') {
      const below = cards[columnIndex + 3]
      next = below
        ? buttons.indexOf(below)
        : Math.min(index + (cards.length - columnIndex || 1), buttons.length - 1)
    } else if (event.key === 'ArrowUp') {
      const above = cards[columnIndex - 3]
      next = above ? buttons.indexOf(above) : index - (columnIndex + 1 || 1)
    } else if (event.key === 'ArrowRight') next = Math.min(index + 1, buttons.length - 1)
    else if (event.key === 'ArrowLeft') next = index - 1
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = buttons.length - 1
    else return
    event.preventDefault()
    if (next < 0) searchRef.current?.focus()
    else buttons[next]?.focus()
  }

  function renderConnection(host: domain.Host) {
    return (
      <li
        key={host.id}
        className="group relative min-w-0 rounded-md border border-border/70 bg-background transition-colors hover:border-border hover:bg-[var(--theme-tab-hover)] focus-within:border-ring"
      >
        <Button
          variant="ghost"
          data-launchpad-nav
          className="h-auto w-full min-w-0 flex-col items-start justify-start gap-1 rounded-md px-3 py-3 text-left font-normal hover:bg-transparent hover:text-[var(--theme-tab-text)]"
          onClick={() => onConnect(host.id)}
          title={`${host.alias} · ${host.user}@${host.hostName}:${host.port}`}
        >
          <span className="flex w-full min-w-0 items-center gap-2 pr-8">
            <Terminal className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span className="truncate text-sm font-medium">{host.alias}</span>
          </span>
          <span className="w-full truncate text-xs text-muted-foreground">
            {host.user}@{host.hostName}
            {host.port !== 22 ? `:${host.port}` : ''}
          </span>
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          disabled={pinBusy}
          className="absolute right-2 top-2 text-muted-foreground hover:bg-[var(--theme-tab-active)] hover:text-[var(--theme-tab-text)]"
          aria-label={`${t(host.favorite ? 'hosts:removeFromFavorites' : 'hosts:addToFavorites')} · ${host.alias}`}
          title={t(host.favorite ? 'hosts:removeFromFavorites' : 'hosts:addToFavorites')}
          onClick={(event) => {
            const row = event.currentTarget.closest('li')
            const neighbor = row?.nextElementSibling ?? row?.previousElementSibling
            const next = neighbor?.querySelector<HTMLButtonElement>('[data-launchpad-nav]')
            ;(next ?? searchRef.current)?.focus()
            onTogglePinned(host.id)
          }}
        >
          {host.favorite ? (
            <PinOff className="size-3.5" aria-hidden="true" />
          ) : (
            <Pin className="size-3.5" aria-hidden="true" />
          )}
        </Button>
      </li>
    )
  }

  return (
    <section
      className="flex h-full min-h-0 flex-col overflow-hidden bg-background"
      aria-label={t('hosts:connections')}
    >
      <header className="shrink-0 px-8 pb-5 pt-6" data-launchpad-header>
        <div className="mx-auto flex w-full max-w-lg flex-col gap-5">
          <div className="flex flex-col items-center gap-2 text-center">
            <img className="size-12 object-contain" src="./appicon.png" alt="" draggable={false} />
            <h1 className="text-lg font-medium tracking-tight">aTerm</h1>
          </div>

          <Button
            ref={searchRef}
            variant="outline"
            className="h-12 w-full justify-start gap-3 rounded-md px-4 font-normal shadow-sm hover:bg-[var(--theme-tab-hover)] hover:text-[var(--theme-tab-text)] dark:hover:bg-[var(--theme-tab-hover)]"
            onClick={onSearch}
            onKeyDown={(event) => {
              if (event.key === 'ArrowDown' && (pinned.length || recent.length)) {
                event.preventDefault()
                connectionsRef.current
                  ?.querySelector<HTMLButtonElement>('[data-launchpad-nav]')
                  ?.focus()
              }
            }}
          >
            <Search className="size-4 text-muted-foreground" aria-hidden="true" />
            <span className="flex-1 text-left text-sm text-muted-foreground">
              {t('hosts:searchHosts')}
            </span>
            <kbd className="rounded border border-border bg-muted/50 px-1.5 py-0.5 font-sans text-xs text-muted-foreground">
              {formatCommandPaletteShortcut(searchShortcut)}
            </kbd>
          </Button>
          <div className="flex flex-wrap items-center justify-center gap-3">
            <Button variant="default" size="sm" onClick={onCreate}>
              <Plus size={14} aria-hidden="true" />
              {t('hosts:addHost')}
            </Button>
            {onLocalTerminal && (
              <Button variant="default" size="sm" onClick={onLocalTerminal}>
                <Terminal size={14} aria-hidden="true" />
                {t('terminal:localTerminal')}
              </Button>
            )}
          </div>
        </div>
      </header>
      <ScrollArea className="min-h-0 flex-1" data-launchpad-scroll>
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-6 pb-6 pt-2">
          {(pinned.length > 0 || recent.length > 0) && (
            <div ref={connectionsRef} className="space-y-6" onKeyDown={navigateConnections}>
              {pinned.length > 0 && (
                <div className="space-y-3">
                  <h2 className="flex items-center gap-2 px-3 text-xs font-medium text-muted-foreground">
                    <Pin className="size-3.5" aria-hidden="true" />
                    {t('hosts:favorites')}
                  </h2>
                  <ul
                    id={pinnedListID}
                    aria-label={t('hosts:favorites')}
                    className="grid grid-cols-3 gap-3"
                  >
                    {visiblePinned.map(renderConnection)}
                  </ul>
                  {pinned.length > 3 && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="w-full gap-1 text-xs font-normal text-muted-foreground hover:bg-[var(--theme-tab-hover)] hover:text-[var(--theme-tab-text)]"
                      data-launchpad-nav
                      aria-expanded={showAllPinned}
                      aria-controls={pinnedListID}
                      onClick={() => setShowAllPinned((expanded) => !expanded)}
                    >
                      {showAllPinned
                        ? t('hosts:collapsePinned')
                        : t('hosts:morePinned', { count: pinned.length - 3 })}
                      <ChevronDown
                        className={`size-3.5 transition-transform ${showAllPinned ? 'rotate-180' : ''}`}
                        aria-hidden="true"
                      />
                    </Button>
                  )}
                </div>
              )}
              {recent.length > 0 && (
                <div className="space-y-3">
                  <h2 className="flex items-center gap-2 px-3 text-xs font-medium text-muted-foreground">
                    <Clock3 className="size-3.5" aria-hidden="true" />
                    {t('hosts:recentlyUsed')}
                  </h2>
                  <ul className="grid grid-cols-3 gap-3" aria-label={t('hosts:recentlyUsed')}>
                    {recent.map(renderConnection)}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>
      </ScrollArea>
    </section>
  )
}
