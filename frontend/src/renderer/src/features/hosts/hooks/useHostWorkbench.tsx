import { t } from '../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter
} from '@/components/ui/dialog'
import { useAppUI } from '@/components/ui/confirm-provider'
import { Save } from 'lucide-react'
import { ScrollArea } from '../../../components/common/ScrollArea'
import { HostEditorForm } from '../components/HostInspector/HostEditorForm'
import { useHostRepository } from './useHostRepository'
import { useDirectoryRepository } from '../../directories/hooks/useDirectoryRepository'
import { useHostEditorActions } from './useHostEditorActions'
import { useDirectoryActions } from '../../directories/hooks/useDirectoryActions'
import { useSecureData } from './useSecureData'
import {
  buildDirectoryTree,
  buildDirectoryOptions
} from '../../directories/model/directoryModel'
import { buildSSHCommand } from '../model/sshCommand'
import { HostDetailsToolbar } from '../components/HostInspector/HostDetailsToolbar'
import { HostDetails } from '../components/HostInspector/HostDetails'
import { HostNotes } from '../components/HostInspector/HostNotes'
import { useHostNotes } from './useHostNotes'
import { BatchAddHostsModal } from '../../../components/hosts/BatchAddHostsModal'
import { desktop } from '../../../api/app'
import type { BusyAction, EditorMode, FormState, ViewID } from '../../../components/common/types'
const emptyForm: FormState = {
  directoryID: '',
  name: '',
  alias: '',
  hostName: '',
  user: '',
  port: '22',
  identityFile: '',
  keySource: 'system',
  passwordAuth: false,
  password: '',
  passwordState: 'none',
  proxyJump: '',
  jumpHostID: '',
  forwardAgent: false,
  compression: false,
  serverAliveInterval: '',
  serverAliveCountMax: '',
  connectTimeout: '',
  remark: '',
  favorite: false
}
export function useHostWorkbench(
  connectHost: (id: string) => void,
  openFiles: (id: string) => void,
  openHost: (id: string) => void,
  renderPluginActions: (id: string) => import("react").ReactNode
) {
  useTranslation()
  const { modal, message, notification } = useAppUI()
  const hostRepo = useHostRepository(),
    dirRepo = useDirectoryRepository()
  const notes = useHostNotes(hostRepo.updateHostNotes)
  const {
    hosts,
    selectedID,
    setSelectedID,
    selectedHostIDs,
    setSelectedHostIDs,
    selectionAnchorID,
    setSelectionAnchorID,
    draggingHostIDs,
    setDraggingHostIDs
  } = hostRepo
  const { directories, activeView, setActiveView, activeDirectoryID, setActiveDirectoryID } =
    dirRepo
  const [busyAction, setBusyAction] = useState<BusyAction>(null)
  const [loading, setLoading] = useState(true)
  const [editorMode, setEditorMode] = useState<EditorMode>('view')
  const [form, setForm] = useState<FormState>(emptyForm)
  const [formErrors, setFormErrors] = useState<Partial<Record<keyof FormState, string>>>({})
  const [inspectorOpen, setInspectorOpen] = useState(false)
  const [batchAddOpen, setBatchAddOpen] = useState(false)
  const nameInputRef = useRef<HTMLInputElement>(null)
  const { ensureSecureDataUnlocked } = useSecureData(setBusyAction)
  const selected = hosts.find((h) => h.id === selectedID) ?? null
  const visibleSelectedIDs = selectedHostIDs.filter((id) => hosts.some((h) => h.id === id))
  const reportSuccess = (text: string) => {
    void message.success(text)
  }
  const reportError = (text: string, title = t('hosts:actionFailed')) => {
    notification.error({ title, description: text })
  }
  async function refreshAll(nextID?: string | null) {
    setLoading(true)
    try {
      const [nextHosts, nextDirs] = await Promise.all([
        hostRepo.loadHosts(),
        dirRepo.loadDirectories()
      ])
      if (nextID !== undefined) {
        const id = nextHosts.some((h) => h.id === nextID) ? nextID : null
        setSelectedID(id)
        setSelectedHostIDs(id ? [id] : [])
      } else if (!selectedID && nextHosts.length) setSelectedID(nextHosts[0].id)
      setSelectedHostIDs((current) => current.filter((id) => nextHosts.some((h) => h.id === id)))
      if (activeDirectoryID && !nextDirs.some((d) => d.id === activeDirectoryID)) {
        setActiveDirectoryID(null)
        setActiveView('all')
      }
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => {
    void refreshAll().catch((error) => reportError(String(error), t('hosts:unableToLoadHosts')))
  }, [])
  const editor = useHostEditorActions({
    modal,
    hosts,
    selected,
    filteredHosts: hosts,
    visibleSelectedIDs,
    selectedID,
    selectionAnchorID,
    activeView,
    activeDirectoryID,
    editorMode,
    form,
    formErrors,
    inspectorOpen,
    batchAddOpen,
    setEditorMode,
    setForm,
    setFormErrors,
    setInspectorOpen,
    setBatchAddOpen,
    setSelectedID,
    setSelectedHostIDs,
    setSelectionAnchorID,
    setBusyAction,
    createHost: hostRepo.createHost,
    updateHost: hostRepo.updateHost,
    deleteHost: hostRepo.deleteHost,
    deleteHosts: hostRepo.deleteHosts,
    refreshAll,
    reportSuccess,
    reportError,
    ensureSecureDataUnlocked
  })
  const directory = useDirectoryActions({
    modal,
    hosts,
    ...dirRepo,
    selectedID,
    draggingHostIDs,
    setDraggingHostIDs,
    setSelectedHostIDs,
    setSelectionAnchorID,
    setBusyAction,
    moveHosts: hostRepo.moveHostsOptimistically,
    reorderHosts: hostRepo.reorderHostsOptimistically,
    refreshAll,
    reportSuccess,
    reportError
  })
  function selectView(view: ViewID) {
    setActiveView(view)
    setActiveDirectoryID(null)
    setInspectorOpen(false)
    setEditorMode('view')
  }
  function selectDirectory(id: string) {
    setSelectedHostIDs([])
    setActiveView('directory')
    setActiveDirectoryID(id)
    setInspectorOpen(false)
    setEditorMode('view')
  }
  async function copySSHCommand(id: string) {
    const h = hosts.find((h) => h.id === id)
    if (!h) return false
    try {
      await desktop.library.copyText(buildSSHCommand(h, hosts))
      reportSuccess(
        h.identityFile.startsWith('aterm-key://')
          ? t('hosts:managedKeyCommandCopied')
          : t('hosts:sshCommandCopied')
      )
      return true
    } catch (error) {
      reportError(String(error))
      return false
    }
  }
  async function getPassword(id: string) {
    try {
      if (!(await ensureSecureDataUnlocked())) return null
      return await desktop.library.getHostPassword(id)
    } catch (error) {
      reportError(String(error), t('hosts:savedPassword'))
      return null
    }
  }
  const directoryOptions = buildDirectoryOptions(directories)
  const sidebarProps = {
    selectedHostID: selectedID,
    hosts,
    loading,
    selectedHostIDs,
    onSelectHost: (id: string, ids: string[]) => {
      setSelectedID(id)
      setSelectedHostIDs(ids)
      setInspectorOpen(false)
    },
    onCreateHost: editor.startCreate,
    onEditHost: editor.startEditHost,
    onRenameHost: editor.renameHost,
    onOpenHost: openHost,
    onConnectHost: connectHost,
    onOpenFiles: openFiles,
    onDeleteHosts: editor.confirmDeleteHosts,
    onToggleFavoriteHost: editor.toggleFavoriteHost,
    onCopySSHCommand: copySSHCommand,
    onBatchAddHosts: editor.openBatchAdd,
    onMoveHosts: directory.moveHostsToDirectory,
    onHostDragStart: directory.startHostDrag,
    onHostDragEnd: directory.finishHostDrag,
    onReorderHosts: directory.reorderHostsInScope,
    activeView,
    activeDirectoryID,
    directoryTree: buildDirectoryTree(directories),
    directoryEditor: dirRepo.directoryEditor,
    directoryDropTarget: dirRepo.directoryDropTarget,
    draggingDirectoryID: dirRepo.draggingDirectoryID,
    draggingHostIDs,
    hostDropTargetID: dirRepo.hostDropTargetID,
    busyAction,
    onSelectView: selectView,
    onSelectDirectory: selectDirectory,
    onStartCreateDirectory: directory.startCreateDirectory,
    onStartRenameDirectory: directory.startRenameDirectory,
    onDirectoryEditorChange: dirRepo.setDirectoryEditor,
    onSaveDirectory: directory.saveDirectory,
    onCancelDirectoryEditor: () => dirRepo.setDirectoryEditor(null),
    onDeleteDirectory: directory.deleteDirectory,
    onDirectoryDragStart: directory.startDirectoryDrag,
    onDirectoryDragOver: directory.updateDirectoryDrop,
    onDirectoryDrop: directory.dropDirectory,
    onDirectoryDragEnd: directory.finishDirectoryDrag,
    onHostDragOver: directory.updateHostDrop,
    onHostDrop: directory.dropHosts
  }
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (
        event.isComposing ||
        event.key !== 'Enter' ||
        event.shiftKey ||
        event.altKey ||
        event.metaKey === event.ctrlKey
      )
        return
      if (inspectorOpen && editorMode !== 'view' && !busyAction) {
        event.preventDefault()
        void editor.saveHost()
      }
    }
    window.addEventListener('keydown', keydown)
    return () => window.removeEventListener('keydown', keydown)
  }, [inspectorOpen, editorMode, busyAction, editor.saveHost])
  const editorOpen =
    inspectorOpen && (editorMode === 'create' || (editorMode === 'edit' && !!selected))
  function renderHostDetails(
    id: string,
    groupID: string,
    actions: {
      connect: () => void
      openFiles: () => void
      keepOpen: () => void
      connecting: boolean
    }
  ) {
    const host = hosts.find((item) => item.id === id)
    if (!host) return <p className="p-4 text-muted-foreground">{t('hosts:thisHostNoLongerExists')}</p>
    return (
      <div key={id} className="h-full min-h-0 min-w-0">
        <ScrollArea className="h-full min-h-0">
          <HostDetails
            actions={
              <HostDetailsToolbar
                favorite={host.favorite}
                disabled={!!busyAction || actions.connecting}
                connecting={actions.connecting}
                pluginActions={renderPluginActions(id)}
                onConnect={actions.connect}
                onEdit={() => editor.startEditHost(id)}
                onOpenFiles={actions.openFiles}
                onCopy={() => void copySSHCommand(id)}
                onFavorite={() => void editor.toggleFavoriteHost(id)}
              />
            }
            host={host}
            jumpHostName={hosts.find((item) => item.id === host.jumpHostID)?.name}
            notes={
              <HostNotes
                notes={host.remark}
                draft={notes.drafts[groupID]}
                onEdit={() => {
                  actions.keepOpen()
                  notes.begin(groupID, id, host.remark)
                }}
                onChange={(value) => notes.change(groupID, value)}
                onCancel={() => notes.discard(groupID)}
                onSave={() => notes.save(groupID)}
              />
            }
            onGetHostPassword={getPassword}
          />
        </ScrollArea>
      </div>
    )
  }
  const overlays = (
    <>
      {notes.closeDialog}
      <BatchAddHostsModal
        open={batchAddOpen}
        onCancel={() => setBatchAddOpen(false)}
        onImported={editor.finishBatchAdd}
      />
      {editorOpen && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !busyAction) editor.cancelInspectorEdit()
          }}
        >
          <DialogContent
            className="host-editor-modal flex max-h-[85vh] flex-col gap-0 p-0 sm:max-w-[min(calc(100vw-2rem),calc(640px*var(--app-layout-scale)))]"
            aria-describedby={undefined}
            showCloseButton={!busyAction}
            onOpenAutoFocus={(event) => {
              // The first tabbable element is the alias help tooltip, not the input.
              event.preventDefault()
              nameInputRef.current?.focus()
            }}
            onPointerDownOutside={(event) => event.preventDefault()}
            onEscapeKeyDown={(event) => {
              if (busyAction) event.preventDefault()
            }}
          >
            <DialogHeader className="shrink-0 border-b border-border px-5 py-4">
              <DialogTitle>
                {editorMode === 'create' ? t('hosts:addHost') : t('hosts:editHost')}
              </DialogTitle>
            </DialogHeader>
            <ScrollArea
              className={cn(
                'inspector-scroll min-h-0 overflow-auto p-4 overflow-y-auto [.host-inspector_&]:p-3 host-editor-modal-scroll',
                '[.host-editor-modal_&]:max-h-[min(65vh,_640px)] [.host-editor-modal_&]:py-4 [.host-editor-modal_&]:px-5 [.host-editor-modal_&]:flex-1',
                '[.host-editor-modal_&]:min-h-0'
              )}
            >
              {editorOpen && (
                <HostEditorForm
                  key={`${editorMode}:${selected?.id ?? 'new'}`}
                  mode={editorMode}
                  nameInputRef={nameInputRef}
                  hostID={editorMode === 'edit' ? selected?.id : undefined}
                  form={form}
                  directories={directoryOptions}
                  hosts={hosts}
                  errors={formErrors}
                  onChange={(next) => {
                    setForm(next)
                    setFormErrors({})
                  }}
                />
              )}
            </ScrollArea>
            <DialogFooter className="shrink-0 border-t border-border px-5 py-3">
              <Button
                variant="outline"
                disabled={!!busyAction}
                onClick={editor.cancelInspectorEdit}
              >
                {t('hosts:cancel')}
              </Button>
              <Button disabled={!!busyAction} onClick={editor.saveHost}>
                <Save size={14} />
                {t('hosts:save')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </>
  )
  function prepareSettings() {
    setActiveView('settings')
    setActiveDirectoryID(null)
    setEditorMode('view')
    setInspectorOpen(false)
  }
  return {
    unlockForConnection: () => ensureSecureDataUnlocked(),
    unlockForData: () => ensureSecureDataUnlocked(),
    refresh: () => void refreshAll().catch((error) => reportError(String(error))),
    prepareSettings,
    overlayOpen: editorOpen || batchAddOpen,
    hosts,
    activeView,
    setActiveView,
    sidebarProps,
    overlays,
    busy: !!busyAction,
    selectView,
    selectDirectory,
    confirmCloseHostNotes: notes.confirmClose,
    discardHostNotes: notes.discard,
    renderHostDetails
  }
}
