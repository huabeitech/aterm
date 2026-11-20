import { t as translate } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Tip } from '@/components/ui/desktop'
import {
  ActionButton,
  TransferProgress,
  TransferPagination,
  Notice
} from '@/components/ui/workbench-controls'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useRef, useState } from 'react'

import {
  AlertCircle,
  Check,
  Clock,
  Download,
  Upload,
  X,
  Maximize2,
  Minimize2,
  Trash2
} from 'lucide-react'
import { desktop } from '../../api/app'
import type { FileTransfer } from '../../../../shared/contracts/files'
import { fileSize, uploadTimeRemaining } from './fileFormat'
import { FileState } from './FileState'

const active = (t: FileTransfer) => t.state === 'conflict' || t.state === 'queued' || t.state === 'running' || t.state === 'paused'
const retryable = (t: FileTransfer) => t.state === 'failed' || t.state === 'cancelled'
const stateLabel = (t: FileTransfer) =>
  ({
    conflict: translate('files:waitingConflict'),
    skipped: translate('files:skipped'),
    paused: translate('files:transferPaused'),
    queued: translate('files:transferQueued'),
    running: translate('files:transferRunning'),
    completed: translate('files:transferCompleted'),
    cancelled: translate('files:transferCancelled'),
    failed: translate('files:transferFailed')
  })[t.state]
const progressLabel = (t: FileTransfer) =>
  t.state === 'skipped' ? stateLabel(t) :
  t.state === 'completed' && t.directory && t.skipped
    ? translate('files:uploadSummary', { uploaded: t.uploaded ?? 0, skipped: t.skipped })
    : t.state === 'completed'
    ? `${translate('files:transferCompleted')} · ${fileSize(t.total)}`
    : `${stateLabel(t)} · ${fileSize(t.transferred)}${t.total ? ` / ${fileSize(t.total)}` : ''}`
export function TransferPanel({
  sessionID,
  transfers,
  ready,
  onResolve,
  onClose
}: {
  sessionID: string
  transfers: FileTransfer[]
  ready: boolean
  onResolve: (id: string) => void
  onClose: () => void
}) {
  useTranslation()
  const [error, setError] = useState('')
  const [filter, setFilter] = useState('all')
  const [page, setPage] = useState(1),
    [size, setSize] = useState(50)
  const [height, setHeight] = useState(300),
    [expanded, setExpanded] = useState(false)
  const [busy, setBusy] = useState(false)
  const pending = useRef(false),
    panel = useRef<HTMLElement>(null)
  const drag = useRef({ y: 0, height: 300 })
  const counts = {
    all: transfers.length,
    active: transfers.filter(active).length,
    failed: transfers.filter((t) => t.state === 'failed').length,
    completed: transfers.filter((t) => (t.state === 'completed' || t.state === 'skipped')).length,
    cancelled: transfers.filter((t) => t.state === 'cancelled').length
  }
  const filtered = transfers.filter(
    (t) => filter === 'all' || (filter === 'active' ? active(t) : t.state === filter || (filter === 'completed' && t.state === 'skipped'))
  )
  const currentPage = Math.min(page, Math.max(1, Math.ceil(filtered.length / size)))
  async function run(action: () => Promise<unknown>) {
    if (pending.current) return
    pending.current = true
    setBusy(true)
    setError('')
    try {
      await action()
    } catch (error) {
      setError(error instanceof Error ? error.message : translate('files:transferOperationFailed'))
    } finally {
      pending.current = false
      setBusy(false)
    }
  }
  function resize(value: number) {
    const maximum = (panel.current?.parentElement?.clientHeight ?? 600) * 0.7
    setHeight(Math.max(180, Math.min(maximum, value)))
  }
  return (
    <section
      ref={panel}
      className={cn(
        'files-transfers relative flex flex-col shrink-0 min-h-45 max-h-[70%] bg-[var(--files-bg)] [border-top:1px_solid_var(--mac-separator)] shadow-[0_-4px_12px_rgb(0_0_0_/_0.025)]',
        '[&.is-expanded]:absolute [&.is-expanded]:inset-0 [&.is-expanded]:z-12 [&.is-expanded]:max-h-none',
        '[&_>_header]:flex [&_>_header]:items-center [&_>_header]:gap-x-2 [&_>_header]:gap-y-1 [&_>_header]:px-3 [&_>_header]:shrink-0 [&_>_header]:text-xs',
        '[&_>_header_>_span]:text-[var(--files-muted)] [&_>_header_>_span]:text-xs [&_>_footer]:flex [&_>_footer]:justify-end',
        '[&_>_footer]:px-4 [&_>_footer]:py-2 [&_>_footer]:[border-top:1px_solid_var(--mac-separator)]',
        `[&_>_footer]:shrink-0${expanded ? ' is-expanded' : ''}`
      )}
      style={expanded ? undefined : { height }}
      aria-label={translate('files:fileTransfers')}
    >
      {!expanded && (
        <div
          className="files-transfer-resizer absolute -top-1 left-0 right-0 h-2 cursor-ns-resize z-2 touch-none hover:bg-[var(--files-selection)] focus-visible:bg-[var(--files-selection)]"
          role="separator"
          aria-label={translate('files:resizeTransfers')}
          aria-orientation="horizontal"
          aria-valuenow={height}
          tabIndex={0}
          onKeyDown={(e) => {
            if (['ArrowUp', 'ArrowDown'].includes(e.key)) {
              e.preventDefault()
              resize(height + (e.key === 'ArrowUp' ? 40 : -40))
            }
          }}
          onPointerDown={(e) => {
            e.preventDefault()
            drag.current = { y: e.clientY, height: panel.current!.clientHeight }
            e.currentTarget.setPointerCapture(e.pointerId)
          }}
          onPointerMove={(e) => {
            if (e.currentTarget.hasPointerCapture(e.pointerId))
              resize(drag.current.height + drag.current.y - e.clientY)
          }}
          onPointerUp={(e) => e.currentTarget.releasePointerCapture(e.pointerId)}
        />
      )}
      <header>
        <Tabs
          value={filter}
          onValueChange={(value) => {
            setFilter(value)
            setPage(1)
          }}
          className="files-transfer-tabs min-w-0 flex-1 gap-0"
        >
          <TabsList
            aria-label={translate('files:transferStatus')}
            variant="line"
            className="files-transfer-tab-list"
          >
            {Object.entries(counts).map(([value, count]) => (
              <TabsTrigger key={value} value={value} className="files-transfer-tab">
                {
                  {
                    all: translate('files:transferAll'),
                    active: translate('files:transferActive'),
                    failed: translate('files:transferFailed'),
                    completed: translate('files:transferCompleted'),
                    cancelled: translate('files:transferCancelled')
                  }[value as keyof typeof counts]
                }{' '}
                ({count})
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className="files-transfer-toolbar ml-auto flex items-center gap-1">
          {transfers.some(retryable) && (
            <ActionButton
              size="sm"
              className="h-8 px-3 text-xs"
              disabled={busy || !ready}
              onClick={() =>
                void run(async () => {
                  for (const t of transfers.filter(retryable))
                    await desktop.files.retry(sessionID, t.id)
                })
              }
            >
              {translate('files:retryAll')}
            </ActionButton>
          )}
          {counts.active > 0 && (
            <ActionButton
              size="sm"
              className="h-8 px-3 text-xs"
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  for (const t of transfers.filter(active))
                    await desktop.files.cancel(sessionID, t.id)
                })
              }
            >
              {translate('files:cancelAll')}
            </ActionButton>
          )}
          {counts.completed > 0 && (
            <Tip title={translate('files:removeCompletedRecordsTransferredFilesAreKept')}>
              <ActionButton
                size="sm"
                className="h-8 px-3 text-xs"
                disabled={busy}
                onClick={() => void run(() => desktop.files.clearCompleted(sessionID))}
              >
                {translate('files:clearCompleted')}
              </ActionButton>
            </Tip>
          )}
          <ActionButton
            variant="ghost"
            className="size-7"
            aria-label={
              expanded ? translate('files:collapseTransfers') : translate('files:expandTransfers')
            }
            icon={expanded ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
            onClick={() => setExpanded(!expanded)}
          />
          <ActionButton
            variant="ghost"
            className="size-7"
            aria-label={translate('files:hideTransfers')}
            icon={<X size={15} />}
            onClick={onClose}
          />
        </div>
      </header>
      {error && (
        <Notice
          className="files-notice"
          tone="error"
          title={translate('files:transferOperationFailed2')}
          description={error}
        />
      )}
      <div className="files-transfer-list flex-1 min-h-0 overflow-auto [overscroll-behavior:contain]">
        {!filtered.length ? (
          <FileState
            title={
              transfers.length
                ? translate('files:noTransfersInThisView')
                : translate('files:readyToTransferFiles')
            }
            description={
              transfers.length
                ? translate('files:chooseAnotherStatusToSeeYourTransfers')
                : translate('files:uploadsAndDownloadsAppearHere')
            }
            icon={<Download size={24} />}
          >
            {transfers.length > 0 && (
              <ActionButton
                onClick={() => {
                  setFilter('all')
                  setPage(1)
                }}
              >
                {translate('files:showAllTransfers')}
              </ActionButton>
            )}
          </FileState>
        ) : (
          filtered.slice((currentPage - 1) * size, currentPage * size).map((t) => (
            <div className="files-transfer" data-state={t.state} key={t.id} data-transfer-id={t.id}>
              <span className="files-transfer-state-icon" aria-hidden>
                {t.state === 'completed' ? (
                  <Check size={18} />
                ) : t.state === 'failed' ? (
                  <AlertCircle size={18} />
                ) : t.state === 'cancelled' ? (
                  <X size={18} />
                ) : t.state === 'queued' ? (
                  <Clock size={18} />
                ) : t.direction === 'upload' ? (
                  <Upload size={18} />
                ) : (
                  <Download size={18} />
                )}
              </span>
              <div className="files-transfer-description min-w-0">
                <strong title={t.name}>{t.name}</strong>
                <details className="files-transfer-paths">
                  <summary title={t.direction === 'upload' ? t.remotePath : t.localPath}>
                    {t.direction === 'upload'
                      ? translate('files:upload')
                      : translate('files:download')}{' '}
                    · …/
                    {(t.direction === 'upload' ? t.remotePath : t.localPath)
                      .split('/')
                      .filter(Boolean)
                      .slice(-2)
                      .join('/')}
                  </summary>
                  <dl>
                    <dt>{translate('files:local')}</dt>
                    <dd>{t.localPath}</dd>
                    <dt>{translate('files:remote')}</dt>
                    <dd>{t.remotePath}</dd>
                  </dl>
                </details>
                {t.error && (
                  <small className="files-transfer-error" title={t.error}>
                    {t.error}
                  </small>
                )}
              </div>
              <div className="files-transfer-progress min-w-0">
                <small>{progressLabel(t)}</small>
                {(t.state === 'running' || t.state === 'paused' || t.state === 'conflict') && (
                  <small className="files-transfer-speed">
                    {fileSize(t.bytesPerSecond ?? 0)}/s
                    {t.direction === 'upload' && <> · {uploadTimeRemaining(t)}</>}
                  </small>
                )}
                {active(t) && (
                  <TransferProgress
                    value={t.total ? Math.min(100, Math.round((t.transferred / t.total) * 100)) : 0}
                    state="normal"
                  />
                )}
              </div>
              <div className="files-transfer-actions flex items-center justify-end gap-1">
                {t.conflict && (
                  <ActionButton size="sm" onClick={() => onResolve(t.id)}>
                    {translate('files:resolveConflict')}
                  </ActionButton>
                )}
                {(t.state === 'running' || t.state === 'paused') && (
                  <ActionButton
                    size="sm"
                    disabled={busy}
                    onClick={() => void run(() => t.state === 'paused'
                      ? desktop.files.resume(sessionID, t.id)
                      : desktop.files.pause(sessionID, t.id))}
                  >
                    {t.state === 'paused' ? translate('files:resume') : translate('files:pause')}
                  </ActionButton>
                )}
                {active(t) ? (
                  <ActionButton
                    size="sm"
                    disabled={busy}
                    onClick={() => void run(() => desktop.files.cancel(sessionID, t.id))}
                  >
                    {translate('files:cancel')}
                  </ActionButton>
                ) : (
                  <>
                    {retryable(t) && (
                      <ActionButton
                        size="sm"
                        disabled={busy || !ready}
                        onClick={() => void run(() => desktop.files.retry(sessionID, t.id))}
                      >
                        {translate('files:retry')}
                      </ActionButton>
                    )}
                    {t.state === 'completed' && t.direction === 'download' && (
                      <ActionButton
                        size="sm"
                        onClick={() => void run(() => desktop.files.reveal(sessionID, t.id))}
                      >
                        {translate('files:showInFolder')}
                      </ActionButton>
                    )}
                    <Tip title={translate('files:removeRecordKeepFiles')}>
                      <ActionButton
                        variant="ghost"
                        size="sm"
                        disabled={busy}
                        aria-label={translate('files:removeTransfer', { name: t.name })}
                        icon={<Trash2 size={14} />}
                        onClick={() =>
                          void run(() => desktop.files.removeTransfer(sessionID, t.id))
                        }
                      />
                    </Tip>
                  </>
                )}
              </div>
            </div>
          ))
        )}
      </div>
      {filtered.length > size && (
        <footer>
          <TransferPagination
            page={currentPage}
            pageSize={size}
            total={filtered.length}
            onPageChange={(next, pageSize) => {
              setPage(next)
              setSize(pageSize)
            }}
          />
        </footer>
      )}
    </section>
  )
}
