type FileDialogTitle = 'newFolder' | 'newFile' | 'renameItem' | 'moveItem' | 'changePermissions'
type FileDialogLabel = 'name' | 'destinationPath' | 'octalPermissions'
import { t, i18n } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import { TableHeader, TableBody, TableRow, TableCell, TableHead } from '@/components/ui/table'
import { Checkbox } from '@/components/ui/checkbox'
import { ButtonGroup } from '@/components/ui/button-group'
import { Button } from '@/components/ui/button'
import { Tip } from '@/components/ui/desktop'
import { useAppUI } from '@/components/ui/confirm-provider'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { ResizableHeaderCell } from '@/shared/table/ResizableHeaderCell'
import {
  ActionButton,
  TextField,
  ActionMenu,
  Notice,
  BusyIndicator,
  WorkbenchDialog
} from '@/components/ui/workbench-controls'
import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useId,
  type KeyboardEvent
} from 'react'

import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  Download,
  Eye,
  FileText,
  Folder,
  FolderInput,
  FolderOpen,
  FolderUp,
  Home,
  KeyRound,
  Link,
  ListFilter,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  Search,
  Settings2,
  Trash2,
  Upload
} from 'lucide-react'
import { desktop } from '../../api/app'
import type {
  FileSession,
  FileTransfer,
  RemoteEntry,
  FileMutation
} from '../../../../shared/contracts/files'

import { fileSize } from './fileFormat'
import { TransferPanel } from './TransferPanel'
import { FileState, FileListLoading } from './FileState'
import './files.css'
import { useMarqueeSelection } from './useMarqueeSelection'

const isImage = (entry: RemoteEntry) =>
  entry.kind === 'file' && /\.(png|jpe?g|gif|webp)$/i.test(entry.name)
const parentPath = (path: string) => path.slice(0, path.lastIndexOf('/')) || '/'
const childPath = (path: string, name: string) => `${path === '/' ? '' : path}/${name}`
type FileColumnKey = 'name' | 'size' | 'type' | 'modified' | 'permissions' | 'owner'
type FileColumnState = Record<FileColumnKey, { show?: boolean }>
const FILE_COLUMNS_STATE_KEY = 'aterm-file-list-columns-v1'
const FILE_COLUMNS_DEFAULT_STATE: FileColumnState = {
  name: { show: true },
  size: { show: true },
  type: { show: false },
  modified: { show: true },
  permissions: { show: false },
  owner: { show: false }
}
const FILE_COLUMN_DEFAULT_WIDTHS: Record<FileColumnKey, number> = {
  name: 320,
  size: 100,
  type: 110,
  modified: 180,
  permissions: 110,
  owner: 110
}
const FILE_SELECTION_COLUMN_WIDTH = 44
const FILE_ACTION_COLUMN_WIDTH = 48
const icon = (entry: RemoteEntry, size = 18) =>
  entry.kind === 'directory' ? (
    <Folder size={size} />
  ) : entry.kind === 'link' ? (
    <Link size={size} />
  ) : (
    <FileText size={size} />
  )
interface Props {
  statusTarget?: HTMLElement | null
  transferRequest?: number
  layoutScale: number
  session: FileSession
  transfers: FileTransfer[]
  onReconnect: () => void
  showHiddenFiles: boolean
  onShowHiddenFilesChange: (show: boolean) => void
}
function readInitialFileColumnsState() {
  if (typeof window === 'undefined') return FILE_COLUMNS_DEFAULT_STATE
  try {
    const saved = JSON.parse(
      window.localStorage.getItem(FILE_COLUMNS_STATE_KEY) ?? '{}'
    ) as Partial<FileColumnState>
    return { ...FILE_COLUMNS_DEFAULT_STATE, ...saved }
  } catch {
    return FILE_COLUMNS_DEFAULT_STATE
  }
}
export function FileWorkspace({
  layoutScale,
  session,
  transfers,
  onReconnect,
  showHiddenFiles,
  onShowHiddenFilesChange,
  statusTarget,
  transferRequest
}: Props) {
  useTranslation()
  const { message, modal } = useAppUI()
  const [preview, setPreview] = useState<{
    entry: RemoteEntry
    dataURL: string
    size: number
  } | null>(null)
  const [zoom, setZoom] = useState(100),
    [dragging, setDragging] = useState(false)
  const [path, setPath] = useState(''),
    [address, setAddress] = useState(''),
    [entries, setEntries] = useState<RemoteEntry[]>([])
  const [editingAddress, setEditingAddress] = useState(false)
  const [query, setQuery] = useState(''),
    [selected, setSelected] = useState<string[]>([])
  useEffect(() => {
    setSelected([])
  }, [showHiddenFiles])
  const [busy, setBusy] = useState(false),
    [transfersOpen, setTransfersOpen] = useState(false)
  useEffect(() => {
    if (transferRequest) setTransfersOpen(true)
  }, [transferRequest])
  const [history, setHistory] = useState<string[]>([]),
    [historyIndex, setHistoryIndex] = useState(-1)
  const [sort, setSort] = useState<{ field: 'name' | 'size' | 'modified'; asc: boolean }>({
    field: 'name',
    asc: true
  })
  const [columnsState, setColumnsState] = useState<FileColumnState>(readInitialFileColumnsState)
  const [columnWidths, setColumnWidths] = useState(FILE_COLUMN_DEFAULT_WIDTHS)
  const [resizedColumn, setResizedColumn] = useState<FileColumnKey | null>(null)
  const resizeWidths = useRef(FILE_COLUMN_DEFAULT_WIDTHS)
  const [dialog, setDialog] = useState<{
    title: FileDialogTitle
    label: FileDialogLabel
    value: string
    run: (value: string) => Promise<unknown>
  } | null>(null)
  const [dialogBusy, setDialogBusy] = useState(false)
  const [dialogError, setDialogError] = useState('')
  const dialogErrorID = useId()
  const [listingError, setListingError] = useState<{ path: string; message: string } | null>(null)
  const selection = useMarqueeSelection(selected, setSelected, (paths) => {
    anchor.current = paths[0]
    focusedPath.current = paths.at(-1)
  })
  const focusedPath = useRef<string>()
  const [tableViewportWidth, setTableViewportWidth] = useState(0)
  const typeAhead = useRef({ text: '', time: 0 })
  const generation = useRef(0),
    anchor = useRef<string>(),
    addressRef = useRef<HTMLInputElement>(null),
    breadcrumbRef = useRef<HTMLDivElement>(null)
  const latest = useRef({ path, history, historyIndex })
  latest.current = { path, history, historyIndex }
  useLayoutEffect(() => {
    if (!editingAddress && breadcrumbRef.current)
      breadcrumbRef.current.scrollLeft = breadcrumbRef.current.scrollWidth
  }, [path, editingAddress])
  const ready = session.state === 'ready'
  async function load(
    destination: string,
    mode: 'push' | 'refresh' | number = 'push',
    fallback?: string
  ) {
    const token = ++generation.current
    setBusy(true)
    setListingError(null)
    try {
      let listing
      try {
        listing = await desktop.files.list(session.id, destination)
      } catch (error) {
        if (!fallback || fallback === destination || token !== generation.current) throw error
        listing = await desktop.files.list(session.id, fallback)
      }
      if (token !== generation.current) return
      setPath(listing.path)
      setAddress(listing.path)
      setEntries(listing.entries)
      setSelected((previous) =>
        mode === 'refresh' ? previous.filter((p) => listing.entries.some((e) => e.path === p)) : []
      )
      if (mode !== 'refresh') {
        anchor.current = undefined
        focusedPath.current = undefined
      }
      if (typeof mode === 'number') setHistoryIndex(mode)
      else if (
        mode === 'push' &&
        latest.current.history[latest.current.historyIndex] !== listing.path
      ) {
        const next = [
          ...latest.current.history.slice(0, latest.current.historyIndex + 1),
          listing.path
        ].slice(-100)
        setHistory(next)
        setHistoryIndex(next.length - 1)
      }
    } catch (error) {
      if (token === generation.current)
        setListingError({
          path: destination,
          message: error instanceof Error ? error.message : t('files:unableToReadThisFolder')
        })
    } finally {
      if (token === generation.current) setBusy(false)
    }
  }
  useEffect(() => {
    if (ready) void load(session.initialPath || session.home, 'push', session.home)
  }, [session.id, ready])
  useEffect(
    () =>
      desktop.files.onEvent((event) => {
        if (event.type === 'saved' && event.sessionID === session.id && latest.current.path) {
          void load(latest.current.path, 'refresh')
        }
      }),
    [session.id]
  )
  useEffect(
    () => () => {
      generation.current++
    },
    []
  )
  useEffect(() => {
    const container = selection.container.current
    if (!container) return
    const resize = () => setTableViewportWidth(container.clientWidth)
    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(container)
    return () => observer.disconnect()
  }, [])
  const completed = transfers.filter((t) => t.state === 'completed').length
  useEffect(() => {
    if (completed && latest.current.path) void load(latest.current.path, 'refresh')
  }, [completed])
  const visible = entries
    .filter(
      (e) =>
        (showHiddenFiles || !e.name.startsWith('.')) &&
        e.name.toLowerCase().includes(query.toLowerCase())
    )
    .sort((a, b) => {
      if ((a.kind === 'directory') !== (b.kind === 'directory'))
        return a.kind === 'directory' ? -1 : 1
      const value =
        sort.field === 'name'
          ? a.name.localeCompare(b.name, undefined, { numeric: true })
          : a[sort.field] - b[sort.field]
      return (sort.asc ? 1 : -1) * value
    })
  const chosen = visible.filter((e) => selected.includes(e.path)),
    single = chosen.length === 1 ? chosen[0] : undefined
  const fileColumns = useMemo(
    () => [
      { key: 'name' as const, title: t('files:name'), minWidth: 180 },
      { key: 'size' as const, title: t('files:size'), minWidth: 72 },
      { key: 'type' as const, title: t('files:type'), minWidth: 84 },
      { key: 'modified' as const, title: t('files:modified'), minWidth: 140 },
      { key: 'permissions' as const, title: t('files:permissions'), minWidth: 88 },
      { key: 'owner' as const, title: t('files:ownerGroup'), minWidth: 96 }
    ],
    [i18n.language]
  )
  const visibleFileColumns = fileColumns.filter((column) => columnsState[column.key].show !== false)
  // Keep the column being dragged at its requested width; another data column fills the viewport.
  const fillColumn =
    (resizedColumn === 'name'
      ? visibleFileColumns.filter((column) => column.key !== 'name').at(-1)
      : null) ?? visibleFileColumns.find((column) => column.key === 'name')!
  const otherColumnsWidth = visibleFileColumns.reduce(
    (total, column) =>
      total + (column.key === fillColumn.key ? 0 : columnWidths[column.key] * layoutScale),
    0
  )
  const resolvedWidths = {
    ...columnWidths,
    [fillColumn.key]: Math.max(
      resizedColumn ? fillColumn.minWidth : columnWidths[fillColumn.key],
      (tableViewportWidth -
        FILE_SELECTION_COLUMN_WIDTH -
        FILE_ACTION_COLUMN_WIDTH -
        otherColumnsWidth) /
        layoutScale
    )
  }
  const tableWidth =
    FILE_SELECTION_COLUMN_WIDTH +
    FILE_ACTION_COLUMN_WIDTH +
    otherColumnsWidth +
    resolvedWidths[fillColumn.key] * layoutScale
  const failedTransfers = transfers.filter((t) => t.state === 'failed').length
  const activeTransfers = transfers.filter((t) => ['running', 'queued'].includes(t.state))
  const seenActiveTransferIDs = useRef(new Set<string>())
  useEffect(() => {
    const activeIDs = new Set(activeTransfers.map((t) => t.id))
    if ([...activeIDs].some((id) => !seenActiveTransferIDs.current.has(id))) {
      setTransfersOpen(true)
    }
    seenActiveTransferIDs.current = activeIDs
  }, [transfers])
  const report = (error: unknown) => {
    void message.error(error instanceof Error ? error.message : t('files:fileOperationFailed'))
  }
  const run = async (operation: () => Promise<unknown>) => {
    try {
      await operation()
    } catch (error) {
      report(error)
    }
  }
  const refresh = () => load(path, 'refresh')
  function startAddressEditing() {
    setAddress(path)
    setEditingAddress(true)
    requestAnimationFrame(() => {
      addressRef.current?.focus()
      addressRef.current?.select()
    })
  }
  function toggleColumn(key: FileColumnKey, show: boolean) {
    const next = { ...columnsState, [key]: { show } }
    setColumnsState(next)
    window.localStorage.setItem(FILE_COLUMNS_STATE_KEY, JSON.stringify(next))
  }
  async function mutate(operation: FileMutation) {
    await desktop.files.mutate(session.id, operation)
    await refresh()
  }
  function prompt(
    title: FileDialogTitle,
    label: FileDialogLabel,
    value: string,
    run: (value: string) => Promise<unknown>
  ) {
    setDialogError('')
    setDialog({ title, label, value, run })
  }
  function create(kind: 'mkdir' | 'touch') {
    prompt(kind === 'mkdir' ? 'newFolder' : 'newFile', 'name', '', (name) =>
      mutate({ kind, parent: path, name })
    )
  }
  function rename(entry: RemoteEntry) {
    prompt('renameItem', 'name', entry.name, (name) => {
      if (!name || name.includes('/') || name === '.' || name === '..')
        return Promise.reject(Error(t('files:enterANameWithoutSlashes')))
      return mutate({
        kind: 'rename',
        path: entry.path,
        destination: childPath(parentPath(entry.path), name)
      })
    })
  }
  function move(entry: RemoteEntry) {
    prompt('moveItem', 'destinationPath', entry.path, (destination) =>
      mutate({ kind: 'rename', path: entry.path, destination })
    )
  }
  function permissions(entry: RemoteEntry) {
    prompt(
      'changePermissions',
      'octalPermissions',
      entry.mode.toString(8).padStart(3, '0'),
      (mode) => mutate({ kind: 'chmod', path: entry.path, mode })
    )
  }
  function remove(items: RemoteEntry[]) {
    modal.confirm({
      title:
        items.length === 1
          ? t('files:deleteItemConfirm', { name: items[0].name })
          : t('files:deleteItemsConfirm', { count: items.length }),
      className: 'file-delete-dialog grid-cols-[minmax(0,1fr)]',
      content: (
        <>
          <p>{t('files:deleteItemsWarning')}</p>
          <div className="file-delete-list mt-3 min-w-0 max-h-[min(180px,30dvh)] overflow-y-auto overscroll-contain text-xs font-mono [overflow-wrap:anywhere]">
            {items.map((e) => (
              <div key={e.path} className="py-1">
                {e.path}
              </div>
            ))}
          </div>
        </>
      ),
      okText: t('files:deletePermanently'),
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          await mutate({ kind: 'delete', paths: items.map((e) => e.path) })
        } catch (error) {
          await refresh()
          report(error)
          throw error
        }
      }
    })
  }
  async function read(entry: RemoteEntry) {
    await run(() => desktop.fileEditor.open(session.id, entry.path))
  }
  function open(entry: RemoteEntry) {
    if (isImage(entry)) {
      void run(async () => {
        const image = await desktop.files.image(session.id, entry.path)
        setZoom(100)
        setPreview({ entry, ...image })
      })
      return
    }
    if (entry.kind === 'directory' || entry.kind === 'link') void load(entry.path)
    else void read(entry)
  }
  const download = (items: RemoteEntry[]) =>
    run(async () => {
      await desktop.files.download(
        session.id,
        items.map((e) => e.path)
      )
      setTransfersOpen(true)
    })
  const copy = (entry: RemoteEntry) => run(() => desktop.library.copyText(entry.path))
  function menu(entry: RemoteEntry) {
    const items = selected.includes(entry.path) && chosen.length > 1 ? chosen : [entry]
    return [
      {
        key: 'open',
        label:
          entry.kind === 'directory' || entry.kind === 'link'
            ? t('files:openFolder')
            : isImage(entry)
              ? t('files:previewImage')
              : t('files:viewEditText'),
        icon:
          entry.kind === 'directory' || entry.kind === 'link' ? (
            <FolderOpen size={15} />
          ) : isImage(entry) ? (
            <Eye size={15} />
          ) : (
            <FileText size={15} />
          ),
        onClick: () => open(entry),
        disabled: items.length !== 1
      },
      {
        key: 'download',
        label:
          items.length > 1
            ? t('files:downloadItems', { count: items.length })
            : t('files:download'),
        icon: <Download size={15} />,
        disabled: items.some((e) => !['file', 'directory'].includes(e.kind)),
        onClick: () => void download(items)
      },
      { type: 'divider' as const },
      {
        key: 'rename',
        label: t('files:rename'),
        icon: <Pencil size={15} />,
        disabled: items.length !== 1,
        onClick: () => rename(entry)
      },
      {
        key: 'move',
        label: t('files:moveTo'),
        icon: <FolderInput size={15} />,
        disabled: items.length !== 1,
        onClick: () => move(entry)
      },
      {
        key: 'mode',
        label: t('files:permissions'),
        icon: <KeyRound size={15} />,
        disabled: items.length !== 1 || entry.kind === 'link',
        onClick: () => permissions(entry)
      },
      {
        key: 'copy',
        label: t('files:copyPath'),
        icon: <Copy size={15} />,
        disabled: items.length !== 1,
        onClick: () => void copy(entry)
      },
      { type: 'divider' as const },
      {
        key: 'delete',
        label: t('files:delete'),
        icon: <Trash2 size={15} />,
        danger: true,
        onClick: () => remove(items)
      }
    ]
  }
  function select(
    entry: RemoteEntry,
    event: { metaKey: boolean; ctrlKey: boolean; shiftKey: boolean }
  ) {
    focusedPath.current = entry.path
    selection.container.current?.focus({ preventScroll: true })
    if (event.shiftKey && anchor.current && visible.some((e) => e.path === anchor.current)) {
      const a = visible.findIndex((e) => e.path === anchor.current),
        b = visible.indexOf(entry)
      const range = visible.slice(Math.min(a, b), Math.max(a, b) + 1).map((e) => e.path)
      setSelected((previous) =>
        event.metaKey || event.ctrlKey ? [...new Set([...previous, ...range])] : range
      )
    } else {
      setSelected((previous) =>
        event.metaKey || event.ctrlKey
          ? previous.includes(entry.path)
            ? previous.filter((p) => p !== entry.path)
            : [...previous, entry.path]
          : [entry.path]
      )
      anchor.current = entry.path
    }
  }
  function keyboard(event: KeyboardEvent) {
    const target = event.target as HTMLElement
    if (target.closest('.files-editor')) return
    if (target.closest('input,textarea,[contenteditable="true"]')) return
    const mac = navigator.platform.toLowerCase().includes('mac')
    const mod = mac ? event.metaKey : event.ctrlKey
    if (mod && event.key.toLowerCase() === 'l') {
      event.preventDefault()
      startAddressEditing()
      return
    }
    if (!selection.container.current?.contains(target) || target.closest('button')) return
    if (mod && event.key.toLowerCase() === 'a') {
      event.preventDefault()
      setSelected(visible.map((e) => e.path))
    } else if ((mod && event.key.toLowerCase() === 'r') || event.key === 'F5') {
      event.preventDefault()
      void refresh()
    } else if (event.key === 'Escape') {
      event.preventDefault()
      selection.cancel()
      setSelected([])
      anchor.current = undefined
      focusedPath.current = undefined
    } else if ((event.key === 'F2' || (mac && event.key === 'Enter' && !mod)) && single) {
      event.preventDefault()
      rename(single)
    } else if (
      (event.key === 'Delete' || (mac && mod && event.key === 'Backspace')) &&
      chosen.length
    ) {
      event.preventDefault()
      remove(chosen)
    } else if (((!mac && event.key === 'Enter') || (mod && event.key === 'ArrowDown')) && single) {
      event.preventDefault()
      open(single)
    } else if (
      ((mod && event.key === 'ArrowUp') || (!mac && event.key === 'Backspace')) &&
      path !== '/'
    ) {
      event.preventDefault()
      void load(parentPath(path))
    } else if (event.altKey && ['ArrowLeft', 'ArrowRight'].includes(event.key)) {
      event.preventDefault()
      const index = historyIndex + (event.key === 'ArrowLeft' ? -1 : 1)
      if (index >= 0 && index < history.length) void load(history[index], index)
    } else if (mod && event.shiftKey && event.key.toLowerCase() === 'c' && chosen.length) {
      event.preventDefault()
      void run(() => desktop.library.copyText(chosen.map((e) => e.path).join('\n')))
    } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) && visible.length) {
      event.preventDefault()
      const current = visible.findIndex((e) => e.path === focusedPath.current)
      const index =
        event.key === 'Home'
          ? 0
          : event.key === 'End'
            ? visible.length - 1
            : Math.max(
                0,
                Math.min(
                  visible.length - 1,
                  current < 0
                    ? event.key === 'ArrowUp'
                      ? visible.length - 1
                      : 0
                    : current + (event.key === 'ArrowDown' ? 1 : -1)
                )
              )
      if (event.shiftKey && !anchor.current) anchor.current = visible[Math.max(0, current)].path
      select(visible[index], { ...event, metaKey: false, ctrlKey: false, shiftKey: event.shiftKey })
      selection.container.current
        ?.querySelectorAll<HTMLElement>('[data-remote-path]')
        [index]?.scrollIntoView({ block: 'nearest' })
    } else if (!mod && !event.altKey && event.key.length === 1 && event.key !== ' ') {
      const now = Date.now()
      typeAhead.current = {
        text:
          (now - typeAhead.current.time < 700 ? typeAhead.current.text : '') +
          event.key.toLowerCase(),
        time: now
      }
      const entry = visible.find((e) => e.name.toLowerCase().startsWith(typeAhead.current.text))
      if (entry) {
        event.preventDefault()
        select(entry, { metaKey: false, ctrlKey: false, shiftKey: false })
        selection.container.current
          ?.querySelectorAll<HTMLElement>('[data-remote-path]')
          [visible.indexOf(entry)]?.scrollIntoView({ block: 'nearest' })
      }
    }
  }
  return (
    <section
      className={`files-workspace flex w-full h-full min-h-0 relative${dragging ? ' is-dropping' : ''}`}
      aria-label={t('files:hostFiles', { host: session.title })}
      onKeyDown={keyboard}
      onDragOver={(event) => {
        if (ready && event.dataTransfer.types.includes('Files')) {
          event.preventDefault()
          event.dataTransfer.dropEffect = 'copy'
          setDragging(true)
        }
      }}
      onDragLeave={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node)) setDragging(false)
      }}
      onDrop={(event) => {
        event.preventDefault()
        setDragging(false)
        if (!ready || !path) return
        const paths = desktop.files.pathsForFiles(Array.from(event.dataTransfer.files))
        if (!paths.length) {
          void message.info(t('files:dropFilesFromYourDesktopOrUseTheUploadsButton'))
          return
        }
        void run(async () => {
          await desktop.files.drop(session.id, path, paths)
          setTransfersOpen(true)
        })
      }}
    >
      {dragging && (
        <div
          className={cn(
            'files-drop-overlay absolute inset-2 z-20 [border:2px_dashed_var(--primary)] bg-[color-mix(in_srgb,_var(--mac-content)_92%,_transparent)]',
            'text-[var(--files-info)] flex items-center justify-center gap-3 pointer-events-none rounded-[8px]'
          )}
        >
          <Upload size={24} />
          {t('files:dropFilesOrFoldersToUpload')}
        </div>
      )}
      <div className="files-main relative flex-1 min-w-0 min-h-0 flex flex-col overflow-hidden">
        <header className="files-header flex shrink-0 flex-col gap-3 border-b border-[var(--mac-separator)] px-4 py-3">
          <div className="files-pathbar flex w-full min-w-0 items-center gap-2">
            <ButtonGroup aria-label={t('files:fileNavigation')}>
              <Tip title={t('files:back')}>
                <ActionButton
                  aria-label={t('files:back')}
                  icon={<ArrowLeft size={16} />}
                  disabled={busy || historyIndex < 1}
                  onClick={() => void load(history[historyIndex - 1], historyIndex - 1)}
                />
              </Tip>
              <Tip title={t('files:forward')}>
                <ActionButton
                  aria-label={t('files:forward')}
                  icon={<ArrowRight size={16} />}
                  disabled={busy || historyIndex >= history.length - 1}
                  onClick={() => void load(history[historyIndex + 1], historyIndex + 1)}
                />
              </Tip>
              <Tip title={t('files:parentFolder')}>
                <ActionButton
                  aria-label={t('files:parentFolder')}
                  icon={<ArrowUp size={16} />}
                  disabled={!ready || !path || path === '/'}
                  onClick={() => void load(parentPath(path))}
                />
              </Tip>
              <Tip title={t('files:homeDirectory')}>
                <ActionButton
                  aria-label={t('files:homeDirectory')}
                  icon={<Home size={16} />}
                  disabled={!ready}
                  onClick={() => void load(session.home)}
                />
              </Tip>
            </ButtonGroup>
            <div className="files-location flex-1 min-w-0">
              {editingAddress ? (
                <TextField
                  ref={addressRef}
                  className="h-8 w-full border-[var(--mac-control-border)] bg-[var(--mac-control)] font-mono text-xs dark:bg-[var(--mac-control)] [&_[data-slot=input-group-addon]]:py-0 [&_[data-slot=input-group-control]]:h-8"
                  aria-label={t('files:remotePath')}
                  leading={<FolderOpen size={15} />}
                  value={address}
                  disabled={!ready}
                  onChange={(e) => setAddress(e.target.value)}
                  onBlur={() => {
                    setAddress(path)
                    setEditingAddress(false)
                  }}
                  onEnter={() => {
                    setEditingAddress(false)
                    void load(address)
                  }}
                />
              ) : (
                <div
                  ref={breadcrumbRef}
                  className={cn(
                    'files-location-breadcrumb flex h-8 min-w-0 items-center gap-1 overflow-x-auto rounded-md border border-[var(--mac-control-border)] bg-[var(--mac-control)] px-3 text-xs text-[var(--mac-text)] shadow-xs [scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
                    'hover:bg-[var(--mac-hover-background)] focus-within:ring-2 focus-within:ring-ring/25'
                  )}
                  onClick={(event) => {
                    if (!(event.target as HTMLElement).closest('button')) startAddressEditing()
                  }}
                >
                  <FolderOpen size={15} className="shrink-0" />
                  <button
                    className="files-crumb shrink-0"
                    onClick={() => {
                      if (path === '/') startAddressEditing()
                      else void load('/')
                    }}
                    disabled={!ready}
                    aria-label={path === '/' ? 'Edit remote path: /' : undefined}
                  >
                    /
                  </button>
                  {path
                    .split('/')
                    .filter(Boolean)
                    .map((part, index, parts) => (
                      <span
                        key={index}
                        className="flex shrink-0 items-center gap-1 whitespace-nowrap"
                      >
                        <ChevronRight size={12} className="shrink-0" />
                        <button
                          className="files-crumb max-w-40 truncate"
                          onClick={() => {
                            if (index === parts.length - 1) startAddressEditing()
                            else void load('/' + parts.slice(0, index + 1).join('/'))
                          }}
                          aria-label={
                            index === parts.length - 1 ? t('files:editPath', { path }) : undefined
                          }
                        >
                          {part}
                        </button>
                      </span>
                    ))}
                </div>
              )}
            </div>
            <Tip title={t('files:refresh')}>
              <ActionButton
                aria-label={t('files:refreshFiles')}
                icon={<RefreshCw size={16} />}
                disabled={!ready || !path}
                busy={busy}
                onClick={() => void refresh()}
              />
            </Tip>
          </div>
          <div className="files-actions flex w-full min-w-0 flex-wrap items-center gap-2">
            <ActionMenu
              actions={[
                { key: 'folder', label: t('files:newFolder'), onClick: () => create('mkdir') },
                { key: 'file', label: t('files:newFile'), onClick: () => create('touch') }
              ]}
            >
              <ActionButton disabled={!ready || !path || busy} icon={<Folder size={15} />}>
                {t('files:new')}
              </ActionButton>
            </ActionMenu>
            <div className="flex">
              <Button
                className="rounded-r-none px-3"
                size="sm"
                disabled={!ready || !path}
                onClick={() =>
                  void run(async () => {
                    await desktop.files.upload(session.id, path)
                    setTransfersOpen(true)
                  })
                }
              >
                <Upload size={15} />
                {t('files:uploads')}
              </Button>
              <ActionMenu
                actions={[
                  {
                    key: 'folder',
                    label: t('files:uploadFolder'),
                    icon: <FolderUp size={15} />,
                    onClick: () =>
                      void run(async () => {
                        await desktop.files.upload(session.id, path, true)
                        setTransfersOpen(true)
                      })
                  }
                ]}
              >
                <Button
                  className="w-7 rounded-l-none border-l border-white/20 px-0"
                  size="sm"
                  aria-label={t('files:uploadOptions')}
                  disabled={!ready || !path}
                >
                  <ChevronDown size={14} />
                </Button>
              </ActionMenu>
            </div>
            {chosen.length > 0 && (
              <ActionButton
                icon={<Download size={15} />}
                disabled={
                  !ready ||
                  !chosen.length ||
                  chosen.some((e) => !['file', 'directory'].includes(e.kind))
                }
                onClick={() => void download(chosen)}
              >
                {t('files:download')}
              </ActionButton>
            )}
            <Button
              size="sm"
              variant="outline"
              aria-label={
                activeTransfers.length
                  ? t('files:transfersProgress', { count: activeTransfers.length })
                  : t('files:transfers')
              }
              aria-expanded={transfersOpen}
              onClick={() => setTransfersOpen((value) => !value)}
              className={cn(
                'relative',
                activeTransfers.length && 'border-[var(--files-info)] text-[var(--files-info)]'
              )}
            >
              {activeTransfers.length ? <BusyIndicator /> : <ListFilter size={15} />}
              {t('files:transfers')}
              {activeTransfers.length > 0 && (
                <span className="min-w-4 rounded-full bg-primary px-1 text-center text-xs leading-4 text-white">
                  {activeTransfers.length}
                </span>
              )}
            </Button>
            {chosen.length > 0 && (
              <ActionButton
                icon={<Trash2 size={15} />}
                disabled={!ready || !chosen.length}
                onClick={() => remove(chosen)}
                aria-label={t('files:deleteSelectedFiles')}
              />
            )}
            <div className="files-view-tools ml-auto flex min-w-0 items-center gap-2">
              <TextField
                className="files-filter w-60 max-w-full min-w-0 text-xs"
                leading={<Search size={14} />}
                data-action="filter-files"
                aria-label={t('files:filterFiles')}
                placeholder={t('files:filterThisFolder')}
                clearable
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value)
                  setSelected([])
                }}
              />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <ActionButton
                    variant="ghost"
                    aria-label={t('files:fileListColumns')}
                    icon={<Settings2 size={16} />}
                  />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>{t('files:viewOptions')}</DropdownMenuLabel>
                  <DropdownMenuCheckboxItem
                    checked={showHiddenFiles}
                    onSelect={(event) => event.preventDefault()}
                    onCheckedChange={onShowHiddenFilesChange}
                  >
                    {t('files:hiddenFiles')}
                  </DropdownMenuCheckboxItem>

                  <DropdownMenuSeparator />
                  <DropdownMenuLabel>{t('files:columns')}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  {fileColumns.map((column) => (
                    <DropdownMenuCheckboxItem
                      key={column.key}
                      checked={columnsState[column.key].show !== false}
                      disabled={column.key === 'name'}
                      onSelect={(event) => event.preventDefault()}
                      onCheckedChange={(show) => toggleColumn(column.key, show)}
                    >
                      {column.title}
                    </DropdownMenuCheckboxItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </header>
        {session.state === 'error' && (
          <Notice
            className="files-notice"
            tone="error"

            title={t('files:connectionInterrupted')}
            description={session.error}
            action={<ActionButton onClick={onReconnect}>{t('files:reconnect')}</ActionButton>}
          />
        )}
        {listingError && (
          <Notice
            className="files-notice"
            tone="error"
            title={t('files:unableToOpenFolder')}
            description={`${listingError.path}: ${listingError.message}`}
            action={
              <ActionButton disabled={busy || !ready} onClick={() => void load(listingError.path)}>
                {t('files:tryAgain')}
              </ActionButton>
            }
          />
        )}
        <div className="files-browser flex-1 flex min-h-0 overflow-hidden">
          <div
            ref={selection.container}
            {...selection.handlers}
            className="files-table-scroll relative select-none flex-1 min-w-0 overflow-auto outline-none"
            tabIndex={0}
            aria-label={t('files:remoteFileList')}
            aria-busy={busy || session.state === 'connecting'}
          >
            {selection.box && (
              <div
                className="files-selection-box absolute z-4 pointer-events-none border border-primary bg-[var(--files-selection)]"
                style={selection.box}
              />
            )}
            {session.state === 'connecting' || (busy && !entries.length) ? (
              <FileListLoading
                title={
                  session.state === 'connecting'
                    ? t('files:connectingHost', { host: session.title })
                    : t('files:loadingFolder')
                }
              />
            ) : (
              <>
                <table className="files-table" style={{ width: tableWidth }}>
                  <colgroup>
                    <col style={{ width: FILE_SELECTION_COLUMN_WIDTH }} />
                    {visibleFileColumns.map((column) => (
                      <col
                        key={column.key}
                        style={{
                          width: resolvedWidths[column.key] * layoutScale
                        }}
                      />
                    ))}
                    <col style={{ width: FILE_ACTION_COLUMN_WIDTH }} />
                  </colgroup>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="files-checkbox">
                        <Checkbox
                          aria-label={t('files:selectAllFiles')}
                          checked={
                            chosen.length > 0 && chosen.length < visible.length
                              ? 'indeterminate'
                              : visible.length > 0 && chosen.length === visible.length
                          }
                          onCheckedChange={(checked) =>
                            setSelected(checked === true ? visible.map((e) => e.path) : [])
                          }
                        />
                      </TableHead>
                      {visibleFileColumns.map((column) => (
                        <ResizableHeaderCell
                          key={column.key}
                          width={resolvedWidths[column.key]}
                          maxWidth={Number.MAX_SAFE_INTEGER}
                          minWidth={column.minWidth}
                          onResizeStart={() => {
                            resizeWidths.current = resolvedWidths
                          }}
                          onResize={(width) => {
                            setResizedColumn(column.key)
                            setColumnWidths({ ...resizeWidths.current, [column.key]: width })
                          }}
                          className={cn(
                            'relative px-3 text-left',
                            column.key === 'size' && 'files-size-header'
                          )}
                        >
                          {(['name', 'size', 'modified'] as string[]).includes(column.key) ? (
                            <button
                              className="cursor-pointer border-0 bg-transparent p-0 [font:inherit] text-inherit"
                              onClick={() =>
                                setSort((previous) => ({
                                  field: column.key as 'name' | 'size' | 'modified',
                                  asc: previous.field === column.key ? !previous.asc : true
                                }))
                              }
                            >
                              {column.title}
                              {sort.field === column.key && <span>{sort.asc ? ' ↑' : ' ↓'}</span>}
                            </button>
                          ) : (
                            column.title
                          )}
                        </ResizableHeaderCell>
                      ))}
                      <TableHead className="files-row-menu" />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {visible.map((entry) => (
                      <ActionMenu key={entry.path} context actions={menu(entry)}>
                        <TableRow
                          data-remote-path={entry.path}
                          aria-selected={selected.includes(entry.path)}
                          className={selected.includes(entry.path) ? 'selected' : ''}
                          onClick={(event) => {
                            if (!event.currentTarget.contains(event.target as Node)) return
                            if ((event.target as HTMLElement).closest('button,input,label')) return
                            select(entry, event)
                          }}
                          onDoubleClick={(event) => {
                            if (!(event.target as HTMLElement).closest('button,input,label'))
                              open(entry)
                          }}
                          onContextMenu={() => {
                            if (!selected.includes(entry.path)) {
                              setSelected([entry.path])
                              anchor.current = entry.path
                            }
                            focusedPath.current = entry.path
                            selection.container.current?.focus({ preventScroll: true })
                          }}
                        >
                          <TableCell className="files-checkbox">
                            <Checkbox
                              aria-label={t('files:selectEntry', { name: entry.name })}
                              checked={selected.includes(entry.path)}
                              onCheckedChange={(checked) => {
                                anchor.current = entry.path
                                focusedPath.current = entry.path
                                selection.container.current?.focus({ preventScroll: true })
                                setSelected((previous) =>
                                  checked === true
                                    ? [...previous, entry.path]
                                    : previous.filter((p) => p !== entry.path)
                                )
                              }}
                            />
                          </TableCell>
                          {visibleFileColumns.map((column) => (
                            <TableCell
                              key={column.key}
                              className={cn(
                                column.key === 'name' ? 'files-entry-name' : 'files-entry-meta',
                                column.key === 'size' && 'files-entry-size',
                                entry.kind
                              )}
                            >
                              {column.key === 'name' ? (
                                <div className="flex min-w-0 items-center gap-2">
                                  <span className="files-entry-icon shrink-0">{icon(entry)}</span>
                                  <strong className="truncate font-medium" title={entry.name}>
                                    {entry.name}
                                  </strong>
                                  {entry.kind === 'link' && (
                                    <small className="shrink-0 text-xs text-[var(--files-muted)]">
                                      {t('files:link')}
                                    </small>
                                  )}
                                </div>
                              ) : column.key === 'size' ? (
                                entry.kind === 'directory' ? (
                                  '—'
                                ) : (
                                  fileSize(entry.size)
                                )
                              ) : column.key === 'type' ? (
                                entry.kind === 'directory' ? (
                                  t('files:folder')
                                ) : entry.kind === 'link' ? (
                                  t('files:symbolicLink')
                                ) : entry.kind === 'file' ? (
                                  t('files:file')
                                ) : (
                                  t('files:other')
                                )
                              ) : column.key === 'modified' ? (
                                new Date(entry.modified).toLocaleString(i18n.language, {
                                  year: 'numeric',
                                  month: 'short',
                                  day: 'numeric',
                                  hour: '2-digit',
                                  minute: '2-digit'
                                })
                              ) : column.key === 'permissions' ? (
                                entry.mode.toString(8).padStart(3, '0')
                              ) : (
                                `${entry.uid} / ${entry.gid}`
                              )}
                            </TableCell>
                          ))}
                          <TableCell className="text-right">
                            <ActionMenu actions={menu(entry)}>
                              <ActionButton
                                variant="ghost"
                                aria-label={t('files:entryActions', { name: entry.name })}
                                icon={<MoreHorizontal size={16} />}
                              />
                            </ActionMenu>
                          </TableCell>
                        </TableRow>
                      </ActionMenu>
                    ))}
                  </TableBody>
                </table>
                {!visible.length && !busy && session.state !== 'error' && !listingError && (
                  <FileState
                    title={query ? t('files:noMatchingFiles') : t('files:thisFolderIsEmpty')}
                    description={
                      query
                        ? t('files:tryAnotherNameOrClearTheFilterToSeeAllFiles')
                        : t('files:uploadFilesOrCreateAFolderToGetStarted')
                    }
                    icon={query ? <Search size={24} /> : undefined}
                  >
                    {query ? (
                      <ActionButton onClick={() => setQuery('')}>
                        {t('files:clearFilter')}
                      </ActionButton>
                    ) : (
                      ready && (
                        <>
                          <ActionButton
                            variant="default"
                            onClick={() =>
                              void run(async () => {
                                await desktop.files.upload(session.id, path)
                                setTransfersOpen(true)
                              })
                            }
                            icon={<Upload size={16} />}
                          >
                            {t('files:uploadFiles')}
                          </ActionButton>
                          <ActionButton onClick={() => create('mkdir')}>
                            {t('files:createAFolder')}
                          </ActionButton>
                        </>
                      )
                    )}
                  </FileState>
                )}
              </>
            )}
          </div>
        </div>
        {statusTarget
          ? createPortal(
              <div className="files-status connection-file-status">
                <span>
                  {t('files:itemCount', { count: visible.length })}
                  {chosen.length ? t('files:selectedCount', { count: chosen.length }) : ''}
                </span>
                <button
                  className={failedTransfers ? 'files-status-error' : undefined}
                  onClick={() => setTransfersOpen((value) => !value)}
                >
                  {failedTransfers ? (
                    <AlertCircle size={16} />
                  ) : activeTransfers.length ? (
                    <BusyIndicator />
                  ) : completed ? (
                    <Check size={16} />
                  ) : (
                    <ListFilter size={16} />
                  )}
                  {failedTransfers
                    ? activeTransfers.length
                      ? t('files:transferSummary', {
                          count: failedTransfers,
                          active: activeTransfers.length
                        })
                      : t('files:failedTransfers', { count: failedTransfers })
                    : activeTransfers.length
                      ? t('files:activeTransfers', { count: activeTransfers.length })
                      : completed
                        ? t('files:completedTransfers', { count: completed })
                        : t('files:noTransfersYet')}
                </button>
              </div>,
              statusTarget
            )
          : null}
        {transfersOpen && (
          <TransferPanel
            sessionID={session.id}
            transfers={transfers}
            ready={ready}
            onClose={() => setTransfersOpen(false)}
          />
        )}
      </div>
      <WorkbenchDialog
        title={dialog ? t(`files:${dialog.title}`) : undefined}
        open={!!dialog}

        busy={dialogBusy}
        submitLabel={
          dialog
            ? ({
                newFolder: t('files:createFolder'),
                newFile: t('files:createFile'),
                renameItem: t('files:rename'),
                moveItem: t('files:move'),
                changePermissions: t('files:applyPermissions')
              }[dialog.title] ?? t('files:apply'))
            : t('files:apply')
        }
        onClose={() => !dialogBusy && setDialog(null)}
        onSubmit={() => {
          if (!dialog || dialogBusy) return
          const value = dialog.value
          const validation =
            dialog.label === 'name' &&
            (!value.trim() || value.includes('/') || value === '.' || value === '..')
              ? t('files:enterANameWithoutSlashes')
              : dialog.label === 'octalPermissions' && !/^[0-7]{3,4}$/.test(value)
                ? t('files:enterThreeOrFourOctalDigitsForExample644')
                : dialog.label === 'destinationPath' && !value.startsWith('/')
                  ? t('files:enterAnAbsoluteDestinationPathStartingWith')
                  : ''
          if (validation) {
            setDialogError(validation)
            return
          }
          setDialogError('')
          setDialogBusy(true)
          void dialog
            .run(dialog.value)
            .then(() => setDialog(null))
            .catch((error) =>
              setDialogError(
                error instanceof Error ? error.message : t('files:unableToSaveChanges')
              )
            )
            .finally(() => setDialogBusy(false))
        }}
      >
        <label className="files-input-label grid gap-2 my-4 mx-0">
          {dialog ? t(`files:${dialog.label}`) : undefined}
          <TextField
            autoFocus
            aria-label={dialog ? t(`files:${dialog.label}`) : undefined}
            aria-invalid={!!dialogError}
            aria-describedby={dialogError ? dialogErrorID : undefined}
            readOnly={dialogBusy}
            value={dialog?.value ?? ''}
            onChange={(e) => {
              setDialogError('')
              setDialog((previous) => previous && { ...previous, value: e.target.value })
            }}
          />
        </label>
        {dialogError && (
          <p id={dialogErrorID} role="alert" className="files-dialog-error">
            {dialogError}
          </p>
        )}
      </WorkbenchDialog>
      <WorkbenchDialog
        title={preview?.entry.name ?? t('files:imagePreview')}
        open={!!preview}
        onClose={() => setPreview(null)}
        width={860}
        actions={
          <div className="files-image-tools flex items-center justify-end gap-2">
            <span>{preview ? fileSize(preview.size) : ''}</span>
            <ActionButton
              aria-label={t('files:zoomOut')}
              disabled={zoom <= 25}
              onClick={() => setZoom(Math.max(25, zoom - 25))}
            >
              −
            </ActionButton>
            <span>{zoom}%</span>
            <ActionButton
              aria-label={t('files:zoomIn')}
              disabled={zoom >= 400}
              onClick={() => setZoom(Math.min(400, zoom + 25))}
            >
              +
            </ActionButton>
            <ActionButton onClick={() => setZoom(100)}>{t('files:reset')}</ActionButton>
            <ActionButton onClick={() => preview && void download([preview.entry])}>
              {t('files:download')}
            </ActionButton>
          </div>
        }
      >
        <div className="files-image-preview h-[60vh] overflow-auto bg-[var(--mac-panel)] p-3 [&_img]:block">
          {preview && (
            <img
              alt={preview.entry.name}
              src={preview.dataURL}
              style={{
                maxWidth: zoom === 100 ? '100%' : 'none',
                maxHeight: zoom === 100 ? '60vh' : 'none',
                transform: zoom === 100 ? undefined : `scale(${zoom / 100})`,
                transformOrigin: 'top left'
              }}
              onError={() => {
                setPreview(null)
                void message.error(t('files:unableToDecodeThisImage'))
              }}
            />
          )}
        </div>
      </WorkbenchDialog>
    </section>
  )
}
