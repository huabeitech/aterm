import { useRef, type KeyboardEvent } from 'react'
import { Clock3, Plus, Search, Terminal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { t } from '../../../../shared/i18n'
import type { domain } from '../../api/types'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { formatCommandPaletteShortcut } from '../common/commandPaletteShortcut'

export function ConnectionWelcome({
  hosts,
  searchShortcut,
  onCreate,
  onSearch,
  onConnect,
  onLocalTerminal
}: {
  hosts: domain.Host[]
  searchShortcut: string
  onCreate: () => void
  onSearch: () => void
  onConnect: (id: string) => void
  onLocalTerminal?: () => void
}) {
  useTranslation()
  const searchRef = useRef<HTMLButtonElement>(null)
  const connectionsRef = useRef<HTMLDivElement>(null)
  const recent = hosts
    .filter((host) => Number.isFinite(Date.parse(host.lastUsedAt)))
    .sort(
      (a, b) =>
        Date.parse(b.lastUsedAt) - Date.parse(a.lastUsedAt) || a.name.localeCompare(b.name)
    )
    .slice(0, 9)

  function navigateConnections(event: KeyboardEvent<HTMLDivElement>) {
    const buttons = Array.from(
      connectionsRef.current?.querySelectorAll<HTMLButtonElement>('[data-launchpad-nav]') ?? []
    )
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
    if (index < 0) return
    let next = index
    if (event.key === 'ArrowDown') next = Math.min(index + 3, buttons.length - 1)
    else if (event.key === 'ArrowUp') next = index - 3
    else if (event.key === 'ArrowRight') next = Math.min(index + 1, buttons.length - 1)
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
        className="min-w-0 rounded-md border border-border/70 bg-background transition-colors hover:border-border hover:bg-[var(--theme-tab-hover)] focus-within:border-ring"
      >
        <Button
          variant="ghost"
          data-launchpad-nav
          className="h-auto w-full min-w-0 flex-col items-start justify-start gap-1 rounded-md px-3 py-3 text-left font-normal hover:bg-transparent hover:text-[var(--theme-tab-text)]"
          onClick={() => onConnect(host.id)}
          title={`${host.name} · ${host.user}@${host.hostName}:${host.port}`}
        >
          <span className="flex w-full min-w-0 items-center gap-2">
            <Terminal className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span className="truncate text-sm font-medium">{host.name}</span>
          </span>
          <span className="w-full truncate text-xs text-muted-foreground">
            {host.user}@{host.hostName}
            {host.port !== 22 ? `:${host.port}` : ''}
          </span>
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
              if (event.key === 'ArrowDown' && recent.length > 0) {
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
          {recent.length > 0 && (
            <div ref={connectionsRef} className="space-y-3" onKeyDown={navigateConnections}>
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
      </ScrollArea>
    </section>
  )
}
