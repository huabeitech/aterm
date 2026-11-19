import { usePluginCatalog } from '../plugins/pluginCatalog'
import { useThemeRevision } from '../../app/themeRegistry'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useAppUI } from '@/components/ui/confirm-provider'
import { ActionButton, TextField, ActionMenu } from '@/components/ui/workbench-controls'
import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { terminalRenderer } from './terminalRenderer'
import { terminalSize, terminalTextChunks } from '../../../../shared/terminalData'
import {
  composingKey,
  deferLinuxIMEKey,
  installWindowsIMEWorkaround,
  wordNavigationSequence
} from './terminalKeyboard'

import { X, ArrowDown, ArrowUp } from 'lucide-react'
import { desktop } from '../../api/app'
import type { TerminalSession } from '../../../../shared/contracts/terminal'
import { terminalTheme } from './themes'
interface Props {
  session: TerminalSession
  visible: boolean
  focused: boolean
  hideStatus?: boolean
  split: boolean
  findRequest: number
  inputBlocked: boolean
  style: CSSProperties
  theme: string
  fontSize: number
  wordNavigation: 'shell' | 'terminal'
  onFocus: () => void
  onClose: () => void
  onReconnect: () => void
}
export function TerminalPane(props: Props) {
  const { i18n } = useTranslation()
  useEffect(() => {
    Terminal.strings.promptLabel = t('terminal:terminalInput')
    Terminal.strings.tooMuchOutput = t('terminal:outputSuppressed')
  }, [i18n.language])
  const { message } = useAppUI(),
    container = useRef<HTMLDivElement>(null),
    terminal = useRef<Terminal>(),
    fit = useRef<FitAddon>(),
    search = useRef<SearchAddon>(),
    findInput = useRef<HTMLInputElement>(null)
  const latest = useRef(props)
  latest.current = props
  const [transfer, setTransfer] = useState<
    import('../../../../shared/contracts/terminal').ZmodemProgress | null
  >(null)
  const hasPluginBars = usePluginCatalog().some((plugin) => plugin.enabled)
  const [finding, setFinding] = useState(false),
    [query, setQuery] = useState(''),
    [matchResult, setMatchResult] = useState<{ resultCount: number; resultIndex: number } | null>(
      null
    ),
    [zoom, setZoom] = useState(0)
  const matches = !matchResult
    ? ''
    : !matchResult.resultCount
      ? t('terminal:noMatches')
      : matchResult.resultCount >= 1000
        ? t('terminal:manyMatches')
        : `${matchResult.resultIndex + 1} / ${matchResult.resultCount}`
  const [gpuEnabled, setGpuEnabled] = useState(true)
  const [searchOptions, setSearchOptions] = useState({
    caseSensitive: false,
    wholeWord: false,
    regex: false
  })
  const [searchError, setSearchError] = useState('')
  const [searchComposing, setSearchComposing] = useState(false)
  const searchTimer = useRef<ReturnType<typeof setTimeout>>()
  const searchActive = useRef(false)
  const configureRenderer = useRef<(enabled: boolean) => void>(() => {})
  const fitNow = useRef(() => {})
  const reactivate = useRef(() => {})
  function report(error: unknown) {
    void message.error(
      error instanceof Error ? error.message : t('terminal:terminalOperationFailed')
    )
  }
  const paste = async () => {
    try {
      const text = await desktop.terminal.readClipboard()
      if (!text) return
      if (text.length > 1024 * 1024) {
        void message.warning(t('terminal:pasteUpTo1MbAtATime'))
        return
      }
      terminal.current?.paste(text)
      requestAnimationFrame(() => {
        if (latest.current.visible && latest.current.focused && !latest.current.inputBlocked)
          terminal.current?.focus()
      })
    } catch (error) {
      report(error)
    }
  }
  const copy = () => {
    const selection = terminal.current?.getSelection()
    if (selection) void desktop.library.copyText(selection).catch(report)
  }
  useEffect(() => {
    if (!container.current) return
    const term = new Terminal({
      fontFamily: 'Menlo, Monaco, Consolas, "Liberation Mono", monospace',
      fontSize: latest.current.fontSize,
      theme: terminalTheme(latest.current.theme),
      cursorBlink: true,
      cursorStyle: 'bar',
      scrollback: 10000,
      // Required by the Unicode provider API used by addon-unicode11.
      allowProposedApi: true,
      // Use xterm's negotiated keyboard protocol (as VS Code does), so TUIs
      // receive modifiers such as Shift+Enter without application key mappings.
      vtExtensions: { kittyKeyboard: true },
      drawBoldTextInBrightColors: true,
      minimumContrastRatio: 1,
      screenReaderMode: false,
      scrollbar: { width: 8 }
    })
    const platform = navigator.platform.toLowerCase().includes('mac')
      ? 'darwin'
      : navigator.platform.toLowerCase().includes('linux')
        ? 'linux'
        : 'win32'
    const disposeIMEWorkaround = installWindowsIMEWorkaround(term, platform)
    const fitting = new FitAddon(),
      searching = new SearchAddon()
    term.loadAddon(fitting)
    term.loadAddon(searching)
    term.loadAddon(new Unicode11Addon())
    term.unicode.activeVersion = '11'
    term.loadAddon(
      new WebLinksAddon((event, url) => {
        if (event.metaKey || event.ctrlKey) void desktop.terminal.openLink(url).catch(report)
      })
    )
    term.open(container.current)
    const renderer = terminalRenderer(
      term,
      () => latest.current.visible && !!container.current?.clientWidth
    )
    renderer.refresh()
    configureRenderer.current = renderer.setEnabled
    terminal.current = term
    fit.current = fitting
    search.current = searching
    let disposed = false,
      attached = false,
      lastSeq = 0,
      queued: { seq: number; data: string }[] = [],
      raf = 0,
      resizeTimer: ReturnType<typeof setTimeout> | undefined,
      lastFit = -Infinity,
      lastSize = { cols: 0, rows: 0 },
      accessibilityUpdated = false
    const write = (seq: number, data: string) => {
      if (seq <= lastSeq) return
      lastSeq = seq
      term.write(data, () => {
        if (!disposed) desktop.terminal.acknowledge(props.session.id, seq)
      })
    }
    const unsubscribe = desktop.terminal.onEvent((event) => {
      if (event.type === 'accessibility') {
        accessibilityUpdated = true
        term.options.screenReaderMode = event.enabled
      }
      if (event.type === 'zmodem' && event.id === props.session.id) setTransfer(event.progress)
      if (event.type === 'data' && event.id === props.session.id) {
        if (attached) write(event.seq, event.data)
        else queued.push(event)
      }
    }, props.session.id)
    void desktop.terminal
      .accessibilityEnabled()
      .then((enabled) => {
        if (!disposed && !accessibilityUpdated) term.options.screenReaderMode = enabled
      })
      .catch(report)
    const notifySize = () => {
      const { cols, rows } = term
      if (!attached || (cols === lastSize.cols && rows === lastSize.rows)) return
      lastSize = { cols, rows }
      desktop.terminal.resize(props.session.id, cols, rows)
    }
    const sizeListener = term.onResize(notifySize)
    const fitSize = () => {
      const proposed = fitting.proposeDimensions()
      if (!proposed || !Number.isFinite(proposed.cols) || !Number.isFinite(proposed.rows)) return
      const size = terminalSize(proposed.cols, proposed.rows)
      if (term.cols !== size.cols || term.rows !== size.rows) {
        term.resize(size.cols, size.rows)
        renderer.refreshAfterResize()
      }
    }
    const resize = () => {
      raf = 0
      if (
        disposed ||
        !latest.current.visible ||
        !container.current ||
        container.current.clientWidth < 32 ||
        container.current.clientHeight < 32
      )
        return
      // Keep the terminal's character grid stable while a drawer is sliding to avoid
      // PTY redraws. Check again even if the final frame causes no ResizeObserver event.
      if (container.current.closest('.workspace-body[data-sidebar-animating]')) {
        raf = requestAnimationFrame(resize)
        return
      }
      lastFit = performance.now()
      try {
        fitSize()
      } catch {
        /* Hidden panes are fitted on activation. */
      }
    }
    // Keep a trailing fit while limiting expensive reflows during window/split drags.
    const scheduleResize = () => {
      if (disposed || raf || resizeTimer) return
      const delay = Math.max(0, 32 - (performance.now() - lastFit))
      if (delay) {
        resizeTimer = setTimeout(() => {
          resizeTimer = undefined
          raf = requestAnimationFrame(resize)
        }, delay)
      } else raf = requestAnimationFrame(resize)
    }
    fitNow.current = scheduleResize
    reactivate.current = () => {
      scheduleResize()
      renderer.refresh()
    }
    let pixelRatio = window.devicePixelRatio
    const onWindowResize = () => {
      if (pixelRatio !== window.devicePixelRatio) {
        pixelRatio = window.devicePixelRatio
        renderer.clearTextureAtlas()
      }
      scheduleResize()
    }
    const onFocus = () => reactivate.current()
    window.addEventListener('focus', onFocus)
    window.addEventListener('resize', onWindowResize)
    const observer = new ResizeObserver(scheduleResize)
    observer.observe(container.current)
    fitSize()
    lastSize = { cols: term.cols, rows: term.rows }
    void desktop.terminal
      .attach(props.session.id, term.cols, term.rows)
      .then((snapshot) => {
        if (disposed) return
        lastSeq = snapshot.seq
        term.write(snapshot.data)
        attached = true
        notifySize()
        for (const event of queued) write(event.seq, event.data)
        queued = []
        if (latest.current.visible && latest.current.focused && !latest.current.inputBlocked)
          term.focus()
      })
      .catch((error) => {
        if (!disposed) report(error)
      })
    const data = term.onData((text) => {
      for (const chunk of terminalTextChunks(text)) desktop.terminal.input(props.session.id, chunk)
    })
    const binary = term.onBinary((text) => {
      // onBinary is a byte string, not UTF-8 text (legacy mouse reports).
      for (let offset = 0; offset < text.length; offset += 16384) {
        const bytes = Uint8Array.from(text.slice(offset, offset + 16384), (char) =>
          char.charCodeAt(0)
        )
        desktop.terminal.inputBytes(props.session.id, bytes)
      }
    })
    const result = searching.onDidChangeResults((result) => setMatchResult(result))
    term.attachCustomKeyEventHandler((event) => {
      const mac = navigator.platform.toLowerCase().includes('mac'),
        mod = mac ? event.metaKey : event.ctrlKey
      if (composingKey(event)) return true
      if (event.type !== 'keydown') return true
      // Some remote shells only echo form feed instead of redrawing the screen.
      // Clear the normal buffer locally, while still delivering Ctrl+L to the
      // shell so it can redraw the prompt and preserve the command being edited.
      // Full-screen applications must retain their own Ctrl+L behavior.
      if (
        event.ctrlKey &&
        !event.altKey &&
        !event.metaKey &&
        !event.shiftKey &&
        event.key.toLowerCase() === 'l' &&
        term.buffer.active.type === 'normal'
      ) {
        term.clear()
      }
      const wordSequence = wordNavigationSequence(
        event,
        latest.current.wordNavigation,
        platform,
        term.buffer.active.type === 'alternate'
      )
      if (wordSequence) {
        event.preventDefault()
        desktop.terminal.input(props.session.id, wordSequence)
        return false
      }
      if (deferLinuxIMEKey(event, platform)) return false
      if (
        mod &&
        (mac || event.shiftKey) &&
        event.key.toLowerCase() === 'c' &&
        term.hasSelection()
      ) {
        event.preventDefault()
        copy()
        return false
      }
      if (mod && (mac || event.shiftKey) && event.key.toLowerCase() === 'v') {
        event.preventDefault()
        void paste()
        return false
      }
      if (mod && event.key.toLowerCase() === 'f') {
        event.preventDefault()
        setFinding(true)
        return false
      }
      if (mod && !event.altKey && ['+', '=', '-', '0'].includes(event.key)) {
        event.preventDefault()
        setZoom((value) =>
          event.key === '0' ? 0 : Math.max(-6, Math.min(12, value + (event.key === '-' ? -1 : 1)))
        )
        return false
      }
      return true
    })
    return () => {
      disposed = true
      unsubscribe()
      observer.disconnect()
      cancelAnimationFrame(raf)
      clearTimeout(resizeTimer)
      window.removeEventListener('focus', onFocus)
      window.removeEventListener('resize', onWindowResize)
      sizeListener.dispose()
      data.dispose()
      binary.dispose()
      result.dispose()
      disposeIMEWorkaround()
      clearTimeout(searchTimer.current)
      renderer.dispose()
      term.dispose()
      terminal.current = undefined
      fitNow.current = () => {}
      reactivate.current = () => {}
      configureRenderer.current = () => {}
      queued = []
    }
  }, [props.session.id])
  const themeRevision = useThemeRevision()
  useEffect(() => {
    if (terminal.current) {
      const theme = terminalTheme(props.theme)
      terminal.current.options.theme =
        finding && query && !searchError
          ? {
              ...theme,
              selectionBackground: '#fbbf24',
              selectionInactiveBackground: '#fbbf24',
              selectionForeground: '#171717'
            }
          : theme
    }
  }, [props.theme, finding, query, searchError, themeRevision])
  useEffect(() => {
    if (terminal.current) {
      terminal.current.options.fontSize = Math.max(8, Math.min(36, props.fontSize + zoom))
      fitNow.current()
    }
  }, [props.fontSize, zoom])
  useEffect(() => {
    if (props.visible) reactivate.current()
  }, [props.visible])
  useEffect(() => {
    if (props.visible) {
      if (props.focused && !finding && !props.inputBlocked) terminal.current?.focus()
    }
  }, [props.visible, props.focused, props.inputBlocked, props.session.state, finding])
  useEffect(() => {
    if (props.findRequest) setFinding(true)
  }, [props.findRequest])
  useEffect(() => {
    if (finding && props.findRequest) findInput.current?.focus()
  }, [finding, props.findRequest])
  function find(back = false) {
    clearTimeout(searchTimer.current)
    runSearch(back)
  }
  function clearSearch() {
    search.current?.clearDecorations()
    search.current?.clearActiveDecoration()
    if (searchActive.current) terminal.current?.clearSelection()
    searchActive.current = false
    setMatchResult(null)
  }
  function closeSearch() {
    clearTimeout(searchTimer.current)
    clearSearch()
    setSearchError('')
    setSearchComposing(false)
    setFinding(false)
  }
  function runSearch(back = false, incremental = false) {
    if (searchComposing) return
    setSearchError('')
    if (!query || !finding) {
      clearSearch()
      return
    }
    if (searchOptions.regex) {
      try {
        new RegExp(query)
      } catch {
        clearSearch()
        setSearchError(t('terminal:invalidRegularExpression'))
        return
      }
    }
    const light = props.theme === 'light' || props.theme === 'solarized-light'
    searchActive.current = true
    // SearchAddon 0.16 caches highlights by term; explicitly invalidate on a
    // fresh/incremental search so changing case/word/regex options recounts.
    if (incremental) search.current?.clearDecorations()
    const options = {
      ...searchOptions,
      incremental,
      decorations: {
        matchBackground: light ? '#c8e1ff' : '#26476b',
        matchBorder: light ? '#0969da' : '#79c0ff',
        matchOverviewRuler: '#58a6ff',
        activeMatchBackground: '#fbbf24',
        activeMatchBorder: light ? '#92400e' : '#fff3c4',
        activeMatchColorOverviewRuler: '#fbbf24'
      }
    }
    if (back) search.current?.findPrevious(query, options)
    else search.current?.findNext(query, options)
  }
  useEffect(() => {
    if (!finding || !query) {
      clearSearch()
      setSearchError('')
      return
    }
    if (searchComposing) return
    searchTimer.current = setTimeout(() => runSearch(false, true), 150)
    return () => clearTimeout(searchTimer.current)
  }, [query, finding, searchOptions, searchComposing, props.theme])
  const palette = terminalTheme(props.theme)
  const chromeStyle = {
    ...props.style,
    background: palette.background,
    '--terminal-bg': palette.background,
    '--terminal-fg': palette.foreground,
    '--terminal-accent': palette.blue,
    '--terminal-success': palette.green,
    '--terminal-warning': palette.yellow,
    '--terminal-error': palette.red
  } as CSSProperties
  const ended = props.session.state === 'exited' || props.session.state === 'error'
  return (
    <section
      className={`terminal-pane absolute flex flex-col min-w-0 min-h-0 pt-0 overflow-hidden${props.focused ? ' is-focused' : ''}`}
      style={chromeStyle}
      onPointerDown={props.onFocus}
      data-session-id={props.session.id}
      aria-label={`${props.session.title} terminal`}
    >
      {props.split && (
        <header
          className={cn(
            'terminal-pane-header flex h-6 shrink-0 items-center gap-2 px-2.5 text-[length:var(--app-text-11)] select-none',
            'bg-[var(--terminal-bg)] text-[color-mix(in_srgb,var(--terminal-fg)_50%,var(--terminal-bg))]',
            '[border-bottom:1px_solid_color-mix(in_srgb,var(--terminal-fg)_8%,var(--terminal-bg))]',
            '[--accent:color-mix(in_srgb,var(--terminal-fg)_8%,var(--terminal-bg))] [--accent-foreground:var(--terminal-fg)] [--ring:var(--terminal-accent)]',
            props.focused &&
              'text-[color-mix(in_srgb,var(--terminal-fg)_85%,var(--terminal-bg))] bg-[color-mix(in_srgb,var(--terminal-fg)_3%,var(--terminal-bg))]'
          )}
        >
          <span className="terminal-pane-title min-w-0 flex-1 truncate" title={props.session.title}>
            {props.session.title}
          </span>
          <ActionButton
            variant="ghost"
            size="icon-xs"
            className="h-5 w-5 shrink-0 opacity-50 hover:opacity-100 focus-visible:opacity-100"
            aria-label={t('terminal:closeTerminalPane')}
            title={t('terminal:closeTerminalPane')}
            icon={<X size={12} />}
            onClick={props.onClose}
          />
        </header>
      )}
      {transfer && (
        <div
          className="terminal-transfer flex items-center gap-2 py-1.5 px-3 text-[#c7cfdf] bg-[#253044] text-[length:var(--app-text-11)] [&_span]:flex-1 [&_span]:overflow-hidden [&_span]:text-ellipsis [&_span]:whitespace-nowrap"
          role="status"
        >
          <span>
            ZMODEM {transfer.direction === 'upload' ? t('files:upload') : t('files:download')} ·{' '}
            {transfer.name} · {Math.round(transfer.transferred / 1024)} /{' '}
            {Math.round(transfer.total / 1024)} KB ·{' '}
            {transfer.error ||
              {
                running: t('files:transferRunning'),
                completed: t('files:transferCompleted'),
                cancelled: t('files:transferCancelled'),
                failed: t('files:transferFailed')
              }[transfer.state]}
          </span>
          {transfer.state === 'running' ? (
            <ActionButton
              size="sm"
              onClick={() => void desktop.terminal.cancelTransfer(props.session.id).catch(report)}
            >
              {t('terminal:cancelTransfer')}
            </ActionButton>
          ) : (
            <ActionButton
              size="sm"
              aria-label={t('terminal:dismissTransfer')}
              onClick={() => setTransfer(null)}
            >
              {t('terminal:dismiss')}
            </ActionButton>
          )}
        </div>
      )}
      {finding && (
        <div
          className={cn(
            'terminal-find absolute right-2 z-4 flex w-[min(400px,calc(100%_-_16px))] flex-col gap-2 rounded-lg border border-border bg-popover p-2 text-popover-foreground shadow-lg',
            props.split ? 'top-8' : 'top-2.5'
          )}
        >
          <TextField
            className="w-full bg-background"
            spellCheck={false}
            aria-label={t('terminal:findInTerminal')}
            aria-invalid={!!searchError}
            ref={findInput}
            autoFocus
            value={query}
            placeholder={t('terminal:findInTerminal')}
            onChange={(e) => {
              setQuery(e.target.value)
            }}
            onCompositionStart={() => setSearchComposing(true)}
            onCompositionEnd={() => setSearchComposing(false)}
            onEnter={(e) => find(e.shiftKey)}
            onKeyDown={(e) => {
              if (e.key === 'Escape' && !composingKey(e.nativeEvent)) closeSearch()
            }}
          />
          <div className="flex flex-wrap items-center gap-1">
            {(
              [
                ['caseSensitive', t('terminal:matchCase'), 'Aa'],
                ['wholeWord', t('terminal:matchWholeWord'), t('terminal:word')],
                ['regex', t('terminal:useRegularExpression'), '.*']
              ] as const
            ).map(([key, label, text]) => (
              <ActionButton
                key={key}
                variant="ghost"
                size="sm"
                aria-label={label}
                title={label}
                aria-pressed={searchOptions[key]}
                className="h-7 rounded-sm px-2 text-xs focus-visible:ring-2"
                style={
                  searchOptions[key]
                    ? {
                        backgroundColor: '#0969da',
                        color: '#ffffff'
                      }
                    : undefined
                }
                onClick={() => setSearchOptions((value) => ({ ...value, [key]: !value[key] }))}
              >
                {text}
              </ActionButton>
            ))}
            <small
              className={cn('ml-auto text-xs tabular-nums', searchError && 'text-destructive')}
              role={searchError ? 'alert' : 'status'}
            >
              {searchError || matches}
            </small>
            <ActionButton
              variant="ghost"
              size="icon-xs"
              disabled={!query || !!searchError || matchResult?.resultCount === 0}
              aria-label={t('terminal:previousMatch')}
              icon={<ArrowUp size={14} />}
              onClick={() => find(true)}
            />
            <ActionButton
              variant="ghost"
              size="icon-xs"
              disabled={!query || !!searchError || matchResult?.resultCount === 0}
              aria-label={t('terminal:nextMatch')}
              icon={<ArrowDown size={14} />}
              onClick={() => find()}
            />
            <ActionButton
              variant="ghost"
              size="icon-xs"
              aria-label={t('terminal:closeTerminalSearch')}
              icon={<X size={14} />}
              onClick={closeSearch}
            />
          </div>
        </div>
      )}
      <ActionMenu
        context
        actions={[
          { key: 'copy', label: t('terminal:copy'), onClick: copy },
          { key: 'paste', label: t('terminal:paste'), onClick: () => void paste() },
          {
            key: 'select',
            label: t('terminal:selectAll'),
            onClick: () => terminal.current?.selectAll()
          },
          {
            key: 'clear',
            label: t('terminal:clearScrollback'),
            onClick: () => terminal.current?.clear()
          },
          { key: 'find', label: t('terminal:find'), onClick: () => setFinding(true) },
          ...(props.session.kind === 'ssh' && hasPluginBars
            ? [
                {
                  key: 'plugin-bars',
                  label: t('settings:pluginsShowStatusBars'),
                  onClick: () =>
                    window.dispatchEvent(
                      new CustomEvent('aterm:show-plugin-bars', { detail: props.session.id })
                    )
                }
              ]
            : []),
          {
            key: 'renderer',
            label: gpuEnabled ? t('terminal:useSoftwareRendering') : t('terminal:useGpuRendering'),
            onClick: () => {
              const enabled = !gpuEnabled
              setGpuEnabled(enabled)
              configureRenderer.current(enabled)
            }
          }
        ]}
      >
        <div
          className="flex flex-1 min-h-0 min-w-0 pt-2 pr-1 pb-1 pl-2.5 overflow-hidden"
          data-allow-global-shortcuts="true"
        >
          {/* FitAddon measures this element's height without subtracting its padding. */}
          <div
            className="terminal-screen flex-1 min-h-0 min-w-0 overflow-hidden [&_.xterm-viewport]:rounded-none"
            ref={container}
          />
        </div>
      </ActionMenu>
      {ended && (
        <div
          className={cn(
            'terminal-exit flex items-center gap-3 shrink-0 py-2 px-3 text-xs text-[var(--terminal-fg)] [&_span]:flex-1',
            'bg-[color-mix(in_srgb,var(--terminal-fg)_6%,var(--terminal-bg))] [border-top:1px_solid_color-mix(in_srgb,var(--terminal-fg)_18%,var(--terminal-bg))]',
            '[&_span]:min-w-0 [&_span]:whitespace-pre-wrap [&_span]:wrap-anywhere [&_span]:max-h-25 [&_span]:overflow-auto',
            '[&.is-error]:bg-[color-mix(in_srgb,var(--terminal-error)_12%,var(--terminal-bg))] [&.is-error]:border-t-[color-mix(in_srgb,var(--terminal-error)_35%,var(--terminal-bg))]',
            `${props.session.state === 'error' ? 'is-error' : ''}`
          )}
          role="status"
        >
          <span>
            {props.session.error ??
              (props.session.exitCode != null
                ? t('terminal:processExitedWithCode', { code: props.session.exitCode })
                : t('terminal:processExited'))}
          </span>
          <ActionButton
            size="sm"
            className="border-[color-mix(in_srgb,var(--terminal-fg)_24%,var(--terminal-bg))] bg-[var(--terminal-bg)] text-[var(--terminal-fg)] hover:bg-[color-mix(in_srgb,var(--terminal-fg)_12%,var(--terminal-bg))] hover:text-[var(--terminal-fg)] focus-visible:ring-[var(--terminal-accent)] dark:border-[color-mix(in_srgb,var(--terminal-fg)_24%,var(--terminal-bg))] dark:bg-[var(--terminal-bg)] dark:hover:bg-[color-mix(in_srgb,var(--terminal-fg)_12%,var(--terminal-bg))]"
            onClick={props.onReconnect}
          >
            {props.session.kind === 'local' ? t('terminal:restartShell') : t('terminal:reconnect')}
          </ActionButton>
        </div>
      )}

      {!props.hideStatus && (
        <footer
          aria-label={t('terminal:terminalSessionStatus')}
          className={cn(
            'terminal-pane-statusbar flex h-[var(--app-statusbar-height)] shrink-0 items-center gap-2 px-2.5 text-[length:var(--app-text-11)] select-none',
            'bg-[var(--terminal-bg)] text-[color-mix(in_srgb,var(--terminal-fg)_60%,var(--terminal-bg))]',
            '[border-top:1px_solid_color-mix(in_srgb,var(--terminal-fg)_10%,var(--terminal-bg))]',
            '[--accent:color-mix(in_srgb,var(--terminal-fg)_8%,var(--terminal-bg))] [--accent-foreground:var(--terminal-fg)] [--ring:var(--terminal-accent)]'
          )}
        >
          <span
            role="img"
            aria-label={
              {
                connecting: t('terminal:sessionConnecting'),
                running: t('terminal:sessionRunning'),
                exited: t('terminal:sessionExited'),
                error: t('terminal:sessionError')
              }[props.session.state]
            }
            className={`terminal-state h-1.5 w-1.5 shrink-0 rounded-full bg-current [&.state-running]:bg-[var(--terminal-success)] [&.state-connecting]:bg-[var(--terminal-warning)] [&.state-error]:bg-[var(--terminal-error)] state-${props.session.state}`}
          />
          {!props.split && (
            <span
              className="terminal-pane-title min-w-0 max-w-1/2 truncate"
              title={props.session.title}
            >
              {props.session.title}
            </span>
          )}
          <span className="terminal-pane-status min-w-0 flex-1 truncate">
            {props.session.state === 'connecting'
              ? t('terminal:connecting')
              : props.session.kind === 'local'
                ? t('terminal:local')
                : !props.split && props.session.alias !== props.session.title
                  ? props.session.alias
                  : 'SSH'}
          </span>
        </footer>
      )}
    </section>
  )
}
