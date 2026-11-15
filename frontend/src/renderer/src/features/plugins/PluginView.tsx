import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import type {
  JsonValue,
  PluginInfo,
  PluginStatus,
  PluginViewDocument
} from '../../../../shared/contracts/plugins'
import type { TerminalSession } from '../../../../shared/contracts/terminal'
import { desktop } from '../../api/app'

import type { OpenPluginView } from './viewActions'

function environment(locale: string, element: HTMLElement) {
  const root = document.documentElement
  const style = getComputedStyle(element)
  const tokens: Record<string, string> = {}
  for (const key of [
    '--terminal-bg',
    '--terminal-fg',
    '--terminal-warning',
    '--background',
    '--foreground',
    '--muted-foreground',
    '--border',
    '--primary',
    '--card',
    '--card-foreground',
    '--accent',
    '--accent-foreground',
    '--muted',
    '--ring',
    '--font-sans',
    '--font-family-sans',
    '--app-font-scale',
    '--app-text-12',
    '--app-text-14',
    '--workspace-background',
    '--mac-text',
    '--mac-secondary-text',
    '--mac-hover-background',
    '--mac-separator',
    '--primary-foreground',
    '--destructive',
    '--popover',
    '--popover-foreground',
    '--input',
    '--secondary',
    '--secondary-foreground',
    '--radius',
    '--app-layout-scale',
    '--font-family-mono'
  ]) {
    const value = style.getPropertyValue(key).trim()
    if (value) tokens[key] = value
  }
  return { locale, tokens, appearance: root.classList.contains('dark') ? 'dark' : 'light' }
}

/** Trusted container. Untrusted HTML never enters the application's DOM. */
export function PluginView({
  plugin,
  session,
  status,
  view,
  panel,
  onClose,
  onCommand,
  active = true,
  workspace = false
}: {
  active?: boolean
  workspace?: boolean
  plugin: PluginInfo
  session: TerminalSession
  status?: PluginStatus
  view: OpenPluginView
  panel: HTMLElement
  onClose: () => void
  onCommand: (command: string, args?: JsonValue) => Promise<JsonValue | void>
}) {
  const { i18n } = useTranslation()
  const frame = useRef<HTMLIFrameElement>(null)
  const [document, setDocument] = useState<PluginViewDocument>()
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const title =
    plugin.manifest.contributes.views?.find((v) => v.id === view.id)?.title ?? plugin.manifest.name
  const latest = useRef({ onClose, onCommand })
  latest.current = { onClose, onCommand }
  const state = useRef({})
  state.current = {
    status,
    context: {
      active,
      invocationId: view.invocationId,
      sessionId: session.id,
      sessionLabel: session.alias || session.title,
      view: view.context ?? null
    },
    environment: environment(i18n.resolvedLanguage ?? i18n.language, panel)
  }
  const publish = () => {
    if (document)
      frame.current?.contentWindow?.postMessage(
        {
          channel: 'aterm-view',
          instanceId: document.instanceId,
          type: 'state',
          state: state.current
        },
        '*'
      )
  }
  useEffect(() => {
    let cancelled = false
    let instance: string | undefined
    setError('')
    void desktop.plugins
      .openView(plugin.manifest.id, session.id, view.id)
      .then((value) => {
        instance = value.instanceId
        if (cancelled) void desktop.plugins.closeView(instance).catch(() => {})
        else setDocument(value)
      })
      .catch((err) => {
        if (!cancelled) setError(String(err))
      })
    return () => {
      cancelled = true
      if (instance) void desktop.plugins.closeView(instance).catch(() => {})
    }
  }, [plugin.manifest.id, plugin.manifest.version, session.id, view.id])
  useEffect(publish, [
    document,
    active,
    status,
    view.context,
    view.invocationId,
    i18n.resolvedLanguage,
    session.alias,
    session.title,
    revision
  ])
  useEffect(() => {
    const observer = new MutationObserver(() => setRevision((n) => n + 1))
    observer.observe(window.document.documentElement, {
      attributes: true,
      attributeFilter: ['class', 'style']
    })
    return () => observer.disconnect()
  }, [])
  useEffect(() => {
    if (!document) return
    let second = 0,
      calls = 0,
      active = 0
    const receive = async (event: MessageEvent) => {
      const data = event.data
      if (
        event.source !== frame.current?.contentWindow ||
        data?.channel !== 'aterm-view' ||
        data.instanceId !== document.instanceId ||
        !Number.isSafeInteger(data.requestId)
      )
        return
      const now = Math.floor(Date.now() / 1000)
      if (second !== now) {
        second = now
        calls = 0
      }
      if (++calls > 50 || active >= 16) return
      active++
      let result: JsonValue | void = null
      let error: string | undefined
      try {
        if (JSON.stringify(data.args).length > 8192) throw Error('View request limit')
        if (data.method === 'command' && typeof data.args?.command === 'string')
          result = await latest.current.onCommand(data.args.command, data.args.args)
        else if (data.method === 'close') latest.current.onClose()
        else throw Error('Unknown view operation')
      } catch (err) {
        error = err instanceof Error ? err.message : 'View operation failed'
      } finally {
        active--
      }
      frame.current?.contentWindow?.postMessage(
        {
          channel: 'aterm-view',
          instanceId: document.instanceId,
          type: 'response',
          requestId: data.requestId,
          result: result ?? null,
          error
        },
        '*'
      )
    }
    window.addEventListener('message', receive)
    return () => window.removeEventListener('message', receive)
  }, [document])
  return createPortal(
    <aside
      className={workspace ? 'plugin-view plugin-workspace-view' : 'plugin-view'}
      aria-label={title}
    >
      {!workspace && (
        <button className="plugin-view-close" aria-label={t('common:close')} onClick={onClose}>
          ×
        </button>
      )}
      {error ? (
        <p role="alert">{error}</p>
      ) : document ? (
        <iframe
          ref={frame}
          title={title}
          src={document.url}
          sandbox="allow-scripts"
          referrerPolicy="no-referrer"
          onLoad={publish}
        />
      ) : (
        <div className="plugin-view-loading" aria-busy="true" />
      )}
    </aside>,
    panel
  )
}
