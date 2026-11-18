import { t } from '../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import type { Dispatch, SetStateAction } from 'react'
import type { ConfirmationService } from '@/components/ui/confirm-provider'
import { domain } from '../../../api/types'
import type { BusyAction, EditorMode, FormState, ViewID } from '../../../components/common/types'
import { updateHostSelection } from '../../../components/hosts/selectionState'
import { formFromHost, formToInput, hostToInput, validateForm } from '../model/hostModel'

const emptyForm: FormState = {
  directoryID: '',
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

interface HostEditorActionsOptions {
  modal: ConfirmationService
  hosts: domain.Host[]
  selected: domain.Host | null
  filteredHosts: domain.Host[]
  visibleSelectedIDs: string[]
  selectedID: string | null
  selectionAnchorID: string | null
  activeView: ViewID
  activeDirectoryID: string | null
  editorMode: EditorMode
  form: FormState
  formErrors: Partial<Record<keyof FormState, string>>
  inspectorOpen: boolean
  batchAddOpen: boolean
  setEditorMode: Dispatch<SetStateAction<EditorMode>>
  setForm: Dispatch<SetStateAction<FormState>>
  setFormErrors: Dispatch<SetStateAction<Partial<Record<keyof FormState, string>>>>
  setInspectorOpen: Dispatch<SetStateAction<boolean>>
  setBatchAddOpen: Dispatch<SetStateAction<boolean>>
  setSelectedID: Dispatch<SetStateAction<string | null>>
  setSelectedHostIDs: Dispatch<SetStateAction<string[]>>
  setSelectionAnchorID: Dispatch<SetStateAction<string | null>>
  setBusyAction: Dispatch<SetStateAction<BusyAction>>
  createHost: (input: domain.HostInput) => Promise<domain.Host>
  updateHost: (id: string, input: domain.HostInput) => Promise<domain.Host>
  deleteHost: (id: string) => Promise<void>
  deleteHosts: (ids: string[]) => Promise<void>
  refreshAll: (selectedID?: string | null) => Promise<void>
  reportSuccess: (text: string) => void
  reportError: (text: string, title?: string) => void
  ensureSecureDataUnlocked: () => Promise<boolean>
}

export function useHostEditorActions(options: HostEditorActionsOptions) {
  useTranslation()
  const {
    editorMode,
    form,
    formErrors,
    inspectorOpen,
    batchAddOpen,
    setEditorMode,
    setForm,
    setFormErrors,
    setInspectorOpen,
    setBatchAddOpen
  } = options

  function selectHost(id: string, selection: { additive?: boolean; range?: boolean } = {}) {
    const next = updateHostSelection({
      visibleHosts: options.filteredHosts,
      selectedIDs: options.visibleSelectedIDs,
      anchorID: options.selectionAnchorID,
      clickedID: id,
      additive: Boolean(selection.additive),
      range: Boolean(selection.range)
    })
    options.setSelectedHostIDs(next.selectedIDs)
    options.setSelectionAnchorID(next.anchorID)
    options.setSelectedID(next.selectedIDs.includes(id) ? id : (next.selectedIDs.at(-1) ?? null))
    setEditorMode('view')
    setInspectorOpen(false)
  }

  function openHost(id: string) {
    if (!options.hosts.some((host) => host.id === id)) {
      options.reportError(t('hosts:thisHostNoLongerExists'))
      return
    }
    options.setSelectedID(id)
    options.setSelectedHostIDs([id])
    options.setSelectionAnchorID(id)
    setEditorMode('view')
    setFormErrors({})
    setInspectorOpen(true)
  }

  function startCreate(directoryID?: string) {
    setForm({
      ...emptyForm,
      directoryID:
        directoryID ?? (options.activeView === 'directory' ? (options.activeDirectoryID ?? '') : '')
    })
    setFormErrors({})
    setEditorMode('create')
    setInspectorOpen(true)
  }

  function openBatchAdd() {
    setEditorMode('view')
    setInspectorOpen(false)
    setBatchAddOpen(true)
  }
  function startEdit() {
    if (options.selected) startEditHost(options.selected.id)
  }
  function startEditHost(id: string) {
    const host = options.hosts.find((item) => item.id === id)
    if (!host) return
    options.setSelectedID(host.id)
    options.setSelectedHostIDs([host.id])
    options.setSelectionAnchorID(host.id)
    setForm(formFromHost(host))
    setFormErrors({})
    setEditorMode('edit')
    setInspectorOpen(true)
  }
  function closeInspector() {
    setEditorMode('view')
    setFormErrors({})
    setInspectorOpen(false)
  }
  function cancelInspectorEdit() {
    setFormErrors({})
    if (editorMode === 'edit') {
      setEditorMode('view')
      setInspectorOpen(false)
      if (options.selected) setForm(formFromHost(options.selected))
      return
    }
    closeInspector()
  }

  async function saveHost() {
    const nextErrors = validateForm(form)
    setFormErrors(nextErrors)
    if (Object.keys(nextErrors).length) {
      options.reportError(t('hosts:reviewTheHighlightedFields'))
      return
    }
    if (saveRequiresSecureDataUnlock(editorMode, form, options.selected)) {
      const unlocked = await options.ensureSecureDataUnlocked()
      if (!unlocked) return
    }
    options.setBusyAction('save')
    try {
      const saved =
        editorMode === 'edit' && options.selected
          ? await options.updateHost(options.selected.id, formToInput(form))
          : await options.createHost(formToInput(form))
      await options.refreshAll(saved.id)
      setEditorMode('view')
      setInspectorOpen(false)
      options.reportSuccess(editorMode === 'edit' ? t('hosts:hostUpdated') : t('hosts:hostCreated'))
    } catch (error) {
      const readable = readableHostError(error)
      setFormErrors(readable.fieldErrors)
      options.reportError(readable.text)
    } finally {
      options.setBusyAction(null)
    }
  }

  async function finishBatchAdd(firstCreatedID: string | null, createdCount: number) {
    await options.refreshAll(firstCreatedID)
    options.reportSuccess(t('hosts:hostsAdded', { count: createdCount }))
  }

  function confirmDeleteSelectedHost() {
    if (options.selected) confirmDeleteHost(options.selected.id)
  }
  function confirmDeleteHost(id: string) {
    const host = options.hosts.find((item) => item.id === id)
    if (!host) return
    options.modal.confirm({
      title: t('hosts:deleteHostConfirm', { name: host.alias }),
      content: t('hosts:thisOnlyRemovesItFromAterm'),
      okText: t('hosts:delete'),
      okButtonProps: { danger: true },
      onOk: async () => {
        if (host.passwordAuth) {
          const unlocked = await options.ensureSecureDataUnlocked()
          if (!unlocked) return
        }
        options.setBusyAction('delete')
        try {
          await options.deleteHost(host.id)
          const next = options.hosts.find((item) => item.id !== host.id)?.id ?? null
          await options.refreshAll(next)
          setEditorMode('view')
          if (options.selectedID === host.id) setInspectorOpen(false)
          options.reportSuccess(t('hosts:hostDeleted'))
        } catch (error) {
          options.reportError(errorText(error))
        } finally {
          options.setBusyAction(null)
        }
      }
    })
  }

  function confirmDeleteHosts(ids: string[]) {
    const deletingIDs = [...new Set(ids)].filter((id) =>
      options.hosts.some((host) => host.id === id)
    )
    if (!deletingIDs.length) return
    options.modal.confirm({
      title: t('hosts:deleteHostsConfirm', { count: deletingIDs.length }),
      content: t('hosts:thisOnlyRemovesThemFromAterm'),
      okText: t('hosts:delete'),
      okButtonProps: { danger: true },
      onOk: async () => {
        if (options.hosts.some((host) => deletingIDs.includes(host.id) && host.passwordAuth)) {
          const unlocked = await options.ensureSecureDataUnlocked()
          if (!unlocked) return
        }
        options.setBusyAction('delete')
        try {
          await options.deleteHosts(deletingIDs)
          const deleted = new Set(deletingIDs)
          const next = options.hosts.find((item) => !deleted.has(item.id))?.id ?? null
          await options.refreshAll(next)
          options.setSelectedHostIDs(next ? [next] : [])
          options.setSelectionAnchorID(next)
          setEditorMode('view')
          if (options.selectedID && deleted.has(options.selectedID)) setInspectorOpen(false)
          options.reportSuccess(t('hosts:hostsDeleted', { count: deletingIDs.length }))
        } catch (error) {
          options.reportError(errorText(error))
        } finally {
          options.setBusyAction(null)
        }
      }
    })
  }

  async function renameHost(id: string, alias: string): Promise<string | null> {
    const host = options.hosts.find((item) => item.id === id)
    if (!host) return t('hosts:thisHostNoLongerExists')
    const errors = validateForm({ ...formFromHost(host), alias })
    if (errors.alias) return errors.alias
    options.setBusyAction('save')
    try {
      await options.updateHost(id, { ...hostToInput(host), alias })
      await options.refreshAll(id)
      options.reportSuccess(t('hosts:hostUpdated'))
      return null
    } catch (error) {
      return readableHostError(error).text
    } finally {
      options.setBusyAction(null)
    }
  }

  async function toggleFavoriteHost(id: string) {
    const host = options.hosts.find((item) => item.id === id)
    if (!host) return
    options.setBusyAction('favorite')
    try {
      options.setSelectedID(host.id)
      const updated = await options.updateHost(host.id, {
        ...hostToInput(host),
        favorite: !host.favorite
      })
      await options.refreshAll(updated.id)
      options.reportSuccess(
        updated.favorite ? t('hosts:addedToFavorites') : t('hosts:removedFromFavorites')
      )
    } catch (error) {
      options.reportError(errorText(error))
    } finally {
      options.setBusyAction(null)
    }
  }
  async function toggleSelectedFavorite() {
    if (options.selected) await toggleFavoriteHost(options.selected.id)
  }

  return {
    editorMode,
    setEditorMode,
    form,
    setForm,
    formErrors,
    setFormErrors,
    inspectorOpen,
    setInspectorOpen,
    batchAddOpen,
    setBatchAddOpen,
    selectHost,
    openHost,
    startCreate,
    openBatchAdd,
    startEdit,
    startEditHost,
    closeInspector,
    cancelInspectorEdit,
    saveHost,
    finishBatchAdd,
    confirmDeleteSelectedHost,
    confirmDeleteHost,
    confirmDeleteHosts,
    renameHost,
    toggleFavoriteHost,
    toggleSelectedFavorite
  }
}

const errorText = (error: unknown) =>
  error instanceof Error ? error.message : String(error || t('hosts:unexpectedError'))

function saveRequiresSecureDataUnlock(
  mode: EditorMode,
  form: FormState,
  selected: domain.Host | null
) {
  if (mode === 'create') return form.passwordAuth
  if (mode !== 'edit') return false
  if (form.passwordAuth && form.password) return true
  return Boolean(selected?.passwordAuth && !form.passwordAuth)
}

function readableHostError(error: unknown): {
  text: string
  fieldErrors: Partial<Record<keyof FormState, string>>
} {
  const text = errorText(error)
  const code = error && typeof error === 'object' && 'code' in error ? error.code : undefined
  const field = (
    {
      aliasIsRequired: 'alias',
      hostAliasFormat: 'alias',
      hostNameIsRequired: 'hostName',
      portMustBeBetween1And65535: 'port',
      hostAliasAlreadyExists: 'alias'
    } as const
  )[code as 'aliasIsRequired']
  if (field) return { text, fieldErrors: { [field]: text } }

  return { text, fieldErrors: {} }
}
