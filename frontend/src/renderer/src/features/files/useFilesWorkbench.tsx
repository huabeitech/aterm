import { remoteFileTarget } from '../../../../shared/contracts/terminal'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useAppUI } from '@/components/ui/confirm-provider'
import { useEffect, useRef, useState } from 'react'

import { desktop } from '../../api/app'
import {
  fileTargetKey,
  type FileTarget,
  type FileSession,
  type FileTransfer
} from '../../../../shared/contracts/files'
import { FileWorkspace } from './FileWorkspace'

const SHOW_HIDDEN_FILES_KEY = 'aterm-show-hidden-files'

export function useFilesWorkbench(
  unlock: () => Promise<boolean>,
  onConnected: () => void,
  layoutScale: number,
  resolveTerminalID: (ownerID: string) => string | undefined
) {
  useTranslation()
  const { message, modal } = useAppUI()
  const [showHiddenFiles, setShowHiddenFiles] = useState(
    () => window.localStorage.getItem(SHOW_HIDDEN_FILES_KEY) === 'true'
  )
  useEffect(() => {
    const syncVisibility = (event: StorageEvent) => {
      if (event.storageArea === window.localStorage &&
          (event.key === SHOW_HIDDEN_FILES_KEY || event.key === null)) {
        setShowHiddenFiles(window.localStorage.getItem(SHOW_HIDDEN_FILES_KEY) === 'true')
      }
    }
    window.addEventListener('storage', syncVisibility)
    return () => window.removeEventListener('storage', syncVisibility)
  }, [])
  function updateHiddenFilesVisibility(show: boolean) {
    setShowHiddenFiles(show)
    window.localStorage.setItem(SHOW_HIDDEN_FILES_KEY, String(show))
  }

  const [sessions, setSessions] = useState<
      (FileSession & { ownerID: string; terminalID?: string })[]
    >([]),
    [transfers, setTransfers] = useState<FileTransfer[]>([])
  const current = useRef({ sessions, transfers, unlock, onConnected, resolveTerminalID })
  current.current = { sessions, transfers, unlock, onConnected, resolveTerminalID }
  const opening = useRef(new Set<string>())
  const discarded = useRef(new Set<string>())
  const terminalTargets = useRef(new Map<string, FileTarget | undefined>())
  useEffect(
    () =>
      desktop.terminal.onEvent((event) => {
        if (event.type !== 'state') return
        const target = remoteFileTarget(event.session)
        terminalTargets.current.set(event.session.id, target)
        for (const session of current.current.sessions) {
          if (
            typeof session.target === 'string' ||
            session.terminalID !== event.session.id ||
            fileTargetKey(session.target) === fileTargetKey(target)
          )
            continue
          void release(session.id)
            .then(() => {
              setSessions((previous) => previous.filter((s) => s.id !== session.id))
              setTransfers((previous) => previous.filter((t) => t.sessionID !== session.id))
            })
            .catch(
              (error) =>
                void message.error(
                  error instanceof Error ? error.message : t('files:unableToCloseConnection')
                )
            )
        }
      }),
    []
  )
  useEffect(() => {
    const preventLoss = (event: BeforeUnloadEvent) => {
      if (current.current.transfers.some((t) => ['queued', 'running'].includes(t.state))) {
        event.preventDefault()
        event.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', preventLoss)
    return () => window.removeEventListener('beforeunload', preventLoss)
  }, [])
  useEffect(
    () =>
      desktop.files.onEvent((event) => {
        if (event.type === 'session') {
          if (discarded.current.has(event.session.id)) return
          setSessions((previous) => [
            ...previous.filter((s) => s.id !== event.session.id),
            {
              ...event.session,
              ownerID: previous.find((s) => s.id === event.session.id)?.ownerID ?? '',
              terminalID: previous.find((s) => s.id === event.session.id)?.terminalID
            }
          ])
          if (event.session.state === 'ready') current.current.onConnected()
        } else if (event.type === 'transfers-removed') {
          const removed = new Set(event.ids)
          setTransfers((previous) => previous.filter((t) => !removed.has(t.id)))
        } else if (event.type === 'transfer' && !discarded.current.has(event.transfer.sessionID))
          setTransfers((previous) => {
            return previous.some((t) => t.id === event.transfer.id)
              ? previous.map((t) => (t.id === event.transfer.id ? event.transfer : t))
              : [...previous, event.transfer]
          })
      }),
    []
  )
  const pending = useRef(new Map<string, Promise<void>>())
  const retired = useRef(new Set<string>())
  const liveSessions = useRef(new Set<string>())
  async function release(id: string) {
    if (!liveSessions.current.delete(id)) return
    discarded.current.add(id)
    try {
      await desktop.files.close(id)
    } catch (error) {
      liveSessions.current.add(id)
      discarded.current.delete(id)
      throw error
    }
  }
  async function open(target: FileTarget, ownerID: string) {
    if (retired.current.has(ownerID)) return
    if (
      current.current.sessions.some(
        (s) => s.ownerID === ownerID && fileTargetKey(s.target) === fileTargetKey(target)
      )
    )
      return
    if (pending.current.has(ownerID)) return pending.current.get(ownerID)
    const request = (async () => {
      try {
        if (
          typeof target === 'string' &&
          (await desktop.terminal.requiresUnlock(target)) &&
          !(await current.current.unlock())
        )
          return
        if (retired.current.has(ownerID)) return
        const terminalID =
          typeof target === 'string'
            ? (current.current.resolveTerminalID(ownerID) ?? ownerID)
            : target.terminalID
        const session = await desktop.files.open(target, terminalID)
        liveSessions.current.add(session.id)
        if (
          retired.current.has(ownerID) ||
          (typeof target !== 'string' &&
            fileTargetKey(terminalTargets.current.get(terminalID)) !== fileTargetKey(target))
        ) {
          await release(session.id)
          return
        }
        for (const previous of current.current.sessions.filter((s) => s.ownerID === ownerID))
          await release(previous.id)
        setSessions((previous) => [
          ...previous.filter((s) => s.ownerID !== ownerID),
          { ...session, ownerID, terminalID }
        ])
      } catch (error) {
        void message.error(error instanceof Error ? error.message : t('files:unableToOpenSftp'))
      }
    })()
    pending.current.set(ownerID, request)
    try {
      await request
    } finally {
      pending.current.delete(ownerID)
    }
  }
  async function close(ownerID: string): Promise<boolean> {
    const session = current.current.sessions.find((s) => s.ownerID === ownerID)
    const id = session?.id
    if (
      id &&
      current.current.transfers.some(
        (t) => t.sessionID === id && ['queued', 'running'].includes(t.state)
      )
    ) {
      const confirmed = await new Promise<boolean>((resolve) =>
        modal.confirm({
          title: t('files:closeConnection'),
          content: t(
            'files:activeFileTransfersWillBeCancelledOpenEditorWindowsWillKeepTheirContents'
          ),
          okText: t('files:closeConnection2'),
          onOk: () => {
            resolve(true)
          },
          onCancel: () => {
            resolve(false)
          }
        })
      )
      if (!confirmed) return false
    }
    retired.current.add(ownerID)
    try {
      await pending.current.get(ownerID)
      if (id) {
        await release(id)
        setSessions((previous) => previous.filter((s) => s.id !== id))
        setTransfers((previous) => previous.filter((t) => t.sessionID !== id))
      }
      return true
    } catch (error) {
      retired.current.delete(ownerID)
      void message.error(
        error instanceof Error ? error.message : t('files:unableToCloseConnection')
      )
      return false
    }
  }
  async function reconnect(id: string) {
    const previous = current.current.sessions.find((s) => s.id === id)
    if (!previous) return
    const key = fileTargetKey(previous.target)!
    if (opening.current.has(key)) return
    opening.current.add(key)
    const request = (async () => {
      try {
        if (
          typeof previous.target === 'string' &&
          (await desktop.terminal.requiresUnlock(previous.target)) &&
          !(await current.current.unlock())
        )
          return
        if (retired.current.has(previous.ownerID)) return
        const next = await desktop.files.open(
          previous.target,
          current.current.resolveTerminalID(previous.ownerID) ?? previous.ownerID
        )
        liveSessions.current.add(next.id)
        if (
          retired.current.has(previous.ownerID) ||
          (typeof previous.target !== 'string' &&
            fileTargetKey(terminalTargets.current.get(previous.target.terminalID)) !==
              fileTargetKey(previous.target))
        ) {
          await release(next.id)
          return
        }
        await release(id)
        if (
          retired.current.has(previous.ownerID) ||
          (typeof previous.target !== 'string' &&
            fileTargetKey(terminalTargets.current.get(previous.target.terminalID)) !==
              fileTargetKey(previous.target))
        ) {
          await release(next.id)
          return
        }
        setSessions((list) => [
          ...list.filter((s) => s.id !== id && s.id !== next.id),
          { ...next, ownerID: previous.ownerID, terminalID: previous.terminalID }
        ])
        setTransfers((list) => list.filter((t) => t.sessionID !== id))
      } catch (error) {
        void message.error(error instanceof Error ? error.message : t('files:reconnectFailed'))
      }
    })()
    pending.current.set(previous.ownerID, request)
    try {
      await request
    } finally {
      pending.current.delete(previous.ownerID)
      opening.current.delete(key)
    }
  }

  return {
    reconnect,
    open,
    close,
    sessions,
    transfers,
    renderPage(ownerID: string, statusTarget: HTMLElement | null, transferRequest: number) {
      const session = sessions.find((s) => s.ownerID === ownerID)
      return session ? (
        <FileWorkspace
          key={session.id}
          layoutScale={layoutScale}
          session={session}
          transfers={transfers.filter((t) => t.sessionID === session.id)}
          statusTarget={statusTarget}
          transferRequest={transferRequest}
          showHiddenFiles={showHiddenFiles}
          onShowHiddenFilesChange={updateHiddenFilesVisibility}
          onReconnect={() => void reconnect(session.id)}
        />
      ) : null
    }
  }
}
