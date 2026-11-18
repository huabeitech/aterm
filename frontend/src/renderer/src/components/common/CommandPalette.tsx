import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { CornerDownLeft } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription
} from '@/components/ui/dialog'
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandItem
} from '@/components/ui/command'
import { domain as service } from '../../api/types'
import { type HostSearchMatches, searchHosts } from '../hosts/hostSearch'
import { paletteMode, searchPaletteCommands, type PaletteCommand } from './paletteSearch'
import { formatCommandPaletteShortcut } from './commandPaletteShortcut'

type CommandPaletteProps = {
  open: boolean
  hosts: service.Host[]
  busy: boolean
  connectionFeaturesAvailable: boolean
  fileFeaturesAvailable?: boolean
  request: { query: string; serial: number }
  commands: PaletteCommand[]
  workspaceTabs: PaletteCommand[]
  onExecute: (run: () => void) => void
  onClose: () => void
  onSelectHost: (id: string) => void
  onOpenFiles: (id: string) => void
}
export function CommandPalette({
  open,
  hosts,
  busy,
  connectionFeaturesAvailable,
  fileFeaturesAvailable = false,
  request,
  commands,
  workspaceTabs,
  onExecute,
  onClose,
  onSelectHost,
  onOpenFiles
}: CommandPaletteProps) {
  useTranslation()
  const [query, setQuery] = useState(request.query)
  const [selected, setSelected] = useState('')
  const [cmdPressed, setCmdPressed] = useState(false)
  const [step, setStep] = useState<
    'ssh' | 'sftp' | 'tabs' | ({ id: string } & NonNullable<PaletteCommand['choices']>) | null
  >(null)
  const previousQuery = useRef('>')
  const previousSelected = useRef('')
  const restoreSelected = useRef('')
  const selectionContext = useRef('')
  const submitted = useRef(false)
  const [recent, setRecent] = useState<string[]>(() => {
    try {
      const stored: unknown = JSON.parse(localStorage.getItem('aterm.commandHistory') ?? '[]')
      return Array.isArray(stored)
        ? stored.filter((id): id is string => typeof id === 'string').slice(0, 20)
        : []
    } catch {
      return []
    }
  })
  const choiceStep = typeof step === 'object' ? step : null
  const commandMode = !step && paletteMode(query) === 'commands'
  const commandQuery = commandMode ? query.slice(1).trimStart() : query
  const commandResults = useMemo(
    () =>
      searchPaletteCommands(
        choiceStep?.items ?? (step === 'tabs' ? workspaceTabs : commands),
        commandQuery,
        choiceStep || step === 'tabs' ? [] : recent
      ),
    [step, choiceStep, workspaceTabs, commands, commandQuery, recent]
  )
  const inputRef = useRef<HTMLInputElement>(null)
  const resultsListRef = useRef<HTMLDivElement>(null)
  const results = useMemo(() => searchHosts(hosts, query).slice(0, 12), [hosts, query])
  useEffect(() => {
    if (open) {
      setQuery(request.query)
      setStep(null)
      setCmdPressed(false)
      submitted.current = false
      const frame = requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.setSelectionRange(request.query.length, request.query.length)
      })
      return () => cancelAnimationFrame(frame)
    }
  }, [open, request])
  useEffect(() => {
    const ids =
      commandMode || step === 'tabs' || choiceStep
        ? commandResults.map((command) => command.id)
        : results.map(({ host }) => host.id)
    const context = JSON.stringify([query, choiceStep?.id ?? step, request.serial])
    const changed = selectionContext.current !== context
    const restored = restoreSelected.current
    selectionContext.current = context
    restoreSelected.current = ''
    setSelected(
      (current) => restored || (!changed && ids.includes(current) ? current : (ids[0] ?? ''))
    )
  }, [results, commandResults, commandMode, step, choiceStep, query, request.serial])
  useLayoutEffect(() => {
    const list = resultsListRef.current
    const item = list?.querySelector<HTMLElement>('[cmdk-item][data-selected="true"]')
    if (!list || !item) return

    const listBounds = list.getBoundingClientRect()
    const itemBounds = item.getBoundingClientRect()
    if (itemBounds.top < listBounds.top) list.scrollTop += itemBounds.top - listBounds.top
    else if (itemBounds.bottom > listBounds.bottom)
      list.scrollTop += itemBounds.bottom - listBounds.bottom
  }, [selected, results, commandResults])
  useEffect(() => {
    if (!open) return
    const down = (event: KeyboardEvent) => setCmdPressed(event.metaKey)
    const reset = () => setCmdPressed(false)
    window.addEventListener('keydown', down, true)
    window.addEventListener('keyup', down, true)
    window.addEventListener('blur', reset)
    return () => {
      window.removeEventListener('keydown', down, true)
      window.removeEventListener('keyup', down, true)
      window.removeEventListener('blur', reset)
    }
  }, [open])
  function remember(id: string) {
    const next = [id, ...recent.filter((item) => item !== id)].slice(0, 20)
    setRecent(next)
    try {
      localStorage.setItem('aterm.commandHistory', JSON.stringify(next))
    } catch {
      /* optional history */
    }
  }
  function selectCommand(command: PaletteCommand) {
    if (submitted.current) return
    if (command.choices) {
      previousQuery.current = query
      previousSelected.current = command.id
      setStep({ id: command.id, ...command.choices })
      setQuery('')
      return
    }
    if (['connect-ssh', 'connect-sftp', 'switch-tab'].includes(command.id)) {
      previousQuery.current = query
      previousSelected.current = command.id
      setStep(command.id === 'switch-tab' ? 'tabs' : command.id === 'connect-sftp' ? 'sftp' : 'ssh')
      setQuery('')
      return
    }
    submitted.current = true
    remember(choiceStep?.id ?? (step === 'tabs' ? 'switch-tab' : command.id))
    onExecute(command.run)
  }
  function select(id: string) {
    if (submitted.current || busy || !connectionFeaturesAvailable) return
    submitted.current = true
    if (step) remember(step === 'sftp' ? 'connect-sftp' : 'connect-ssh')
    const files = step === 'sftp' || (!step && cmdPressed && fileFeaturesAvailable)
    onExecute(() => (files ? onOpenFiles(id) : onSelectHost(id)))
  }
  function back() {
    restoreSelected.current = previousSelected.current
    setStep(null)
    setQuery(previousQuery.current)
    requestAnimationFrame(() => inputRef.current?.focus())
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
    >
      <DialogContent
        className="host-command-palette top-20 overflow-hidden p-0 translate-y-0 duration-75 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:animate-none sm:max-w-[680px]"
        showCloseButton={false}
        overlayClassName="bg-transparent duration-75 motion-reduce:animate-none"
        onEscapeKeyDown={(event) => {
          if (step) {
            event.preventDefault()
            back()
          }
        }}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <DialogHeader className="sr-only">
          <DialogTitle>
            {commandMode ? t('commands:commandPalette') : t('commands:search')}
          </DialogTitle>
          <DialogDescription>
            {t('commands:searchHostsOrTypeGtToRunAnApplicationCommand')}
          </DialogDescription>
        </DialogHeader>
        <Command
          shouldFilter={false}
          value={selected}
          onValueChange={setSelected}
          className="host-command-palette-command"
        >
          {step && (
            <div className="flex items-center gap-2 px-4 py-2 text-sm text-muted-foreground">
              <button type="button" onClick={back}>
                {t('commands:back')}
              </button>
              <span>
                {choiceStep
                  ? choiceStep.title
                  : step === 'tabs'
                    ? t('commands:switchWorkspaceTab')
                    : step === 'sftp'
                      ? t('commands:openSftp')
                      : t('commands:connectSsh')}
              </span>
            </div>
          )}
          <CommandInput
            ref={inputRef}
            className="host-command-palette-input"
            aria-label={commandMode ? t('commands:searchCommands') : t('commands:search')}
            placeholder={
              choiceStep
                ? t('commands:searchCategory', { category: choiceStep.title })
                : step === 'tabs'
                  ? t('commands:searchOpenTabs')
                  : commandMode
                    ? t('commands:typeACommand')
                    : t('commands:searchHostsByNameAddressOrNote')
            }
            value={query}
            onValueChange={setQuery}
            autoCapitalize="none"
            autoCorrect="off"
            autoComplete="off"
          />
          <CommandList
            ref={resultsListRef}
            className="host-command-palette-list"
            label={
              choiceStep
                ? choiceStep.title
                : commandMode
                  ? t('commands:commandSearchResults')
                  : step === 'tabs'
                    ? t('commands:workspaceTabs')
                    : t('commands:hostSearchResults')
            }
          >
            <CommandEmpty>
              {choiceStep
                ? t('commands:noThemesFound')
                : commandMode
                  ? t('commands:noCommandsFound')
                  : step === 'tabs'
                    ? t('commands:noTabsFound')
                    : t('commands:noHostsFound')}
            </CommandEmpty>
            <div>
              {commandMode || step === 'tabs' || choiceStep
                ? commandResults.map((command) => (
                    <CommandItem
                      key={command.id}
                      value={command.id}
                      className="host-command-palette-result"
                      onSelect={() => selectCommand(command)}
                    >
                      <span className="host-command-palette-content">
                        <strong className="host-command-palette-title">
                          {highlightCommand(command.label, commandQuery)}
                        </strong>
                        {command.detail && (
                          <span className="host-command-palette-details">{command.detail}</span>
                        )}
                        {!command.detail && !commandQuery && recent.includes(command.id) && (
                          <span className="host-command-palette-details">
                            {t('commands:recentlyUsed')}
                          </span>
                        )}
                      </span>
                      {command.shortcut && (
                        <kbd className="text-xs text-muted-foreground">
                          {formatCommandPaletteShortcut(command.shortcut)}
                        </kbd>
                      )}
                    </CommandItem>
                  ))
                : results.map(({ host, matches }) => {
                    const active = selected === host.id
                    const files =
                      step === 'sftp' || (!step && active && cmdPressed && fileFeaturesAvailable)
                    return (
                      <CommandItem
                        key={host.id}
                        value={host.id}
                        disabled={busy || !connectionFeaturesAvailable}
                        className="host-command-palette-result"
                        onSelect={() => select(host.id)}
                        aria-label={t('commands:openHost', {
                          host: host.name,
                          protocol: files ? 'SFTP' : 'SSH'
                        })}
                      >
                        <span className="host-command-palette-content">
                          <strong className="host-command-palette-title">
                            {highlightText(host.name, host.name === host.alias ? matches.alias : matches.name)}
                          </strong>
                          <span className="host-command-palette-details">
                            <span className="host-command-palette-endpoint">
                              {highlightEndpoint(host, matches)}
                            </span>
                            {host.alias && host.alias !== host.name && matches.alias.length > 0 && (
                              <>
                                <span className="host-command-palette-dot" aria-hidden="true">·</span>
                                <span>{highlightText(host.alias, matches.alias)}</span>
                              </>
                            )}
                            {host.remark && (
                              <>
                                <span className="host-command-palette-dot" aria-hidden="true">
                                  ·
                                </span>
                                <span className="host-command-palette-remark">
                                  {highlightText(host.remark, matches.remark)}
                                </span>
                              </>
                            )}
                          </span>
                        </span>
                        {active && (
                          <span className="host-command-palette-action" aria-hidden="true">
                            <span>{files ? 'SFTP' : 'SSH'}</span>
                            <kbd>
                              {files && '⌘'}
                              <CornerDownLeft />
                            </kbd>
                          </span>
                        )}
                      </CommandItem>
                    )
                  })}
            </div>
          </CommandList>
          <div className="host-command-palette-footer">
            <span>
              <kbd>↑</kbd>
              <kbd>↓</kbd> {t('commands:navigate')}
            </span>
            <span>
              <kbd>↵</kbd>{' '}
              {choiceStep
                ? t('commands:apply')
                : commandMode
                  ? t('commands:run')
                  : step === 'tabs'
                    ? t('commands:switch')
                    : step === 'sftp'
                      ? t('commands:openFiles')
                      : t('commands:connect')}
            </span>
            {fileFeaturesAvailable && !commandMode && !step && (
              <span>
                <kbd>⌘↵</kbd> {t('commands:files')}
              </span>
            )}
            <span className="host-command-palette-dismiss">
              <kbd>Esc</kbd> {step ? t('commands:back2') : t('commands:close')}
            </span>
          </div>
        </Command>
      </DialogContent>
    </Dialog>
  )
}

function hostSummary(host: service.Host) {
  const endpoint = `${host.user ? `${host.user}@` : ''}${host.hostName}${host.port && host.port !== 22 ? `:${host.port}` : ''}`
  return endpoint || host.hostName || t('commands:noAddress')
}

function highlightEndpoint(host: service.Host, matches: HostSearchMatches) {
  const endpoint = hostSummary(host)
  const offset = host.user ? host.user.length + 1 : 0
  const indices = matches.hostName.map((index) => index + offset)
  return highlightText(endpoint, indices)
}

function highlightText(value: string, indices: number[]) {
  if (!indices.length) return value

  const highlighted = new Set(indices)
  const parts: Array<{ text: string; highlighted: boolean }> = []
  for (let index = 0; index < value.length; index += 1) {
    const isHighlighted = highlighted.has(index)
    const previous = parts[parts.length - 1]
    if (previous && previous.highlighted === isHighlighted) {
      previous.text += value[index]
    } else {
      parts.push({ text: value[index], highlighted: isHighlighted })
    }
  }

  return parts.map((part, index) =>
    part.highlighted ? (
      <mark key={index} className="host-command-palette-match">
        {part.text}
      </mark>
    ) : (
      part.text
    )
  )
}

function highlightCommand(label: string, query: string) {
  const indices = new Set<number>()
  const lower = label.toLowerCase()
  for (const token of query.toLowerCase().split(/\s+/).filter(Boolean)) {
    const start = lower.indexOf(token)
    if (start >= 0) for (let i = start; i < start + token.length; i++) indices.add(i)
    else {
      let from = 0
      for (const char of token) {
        const index = lower.indexOf(char, from)
        if (index < 0) break
        indices.add(index)
        from = index + 1
      }
    }
  }
  return highlightText(label, [...indices])
}
