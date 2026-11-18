import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmationDialog } from '@/components/ui/confirmation-dialog'
import { useAppUI } from '@/components/ui/confirm-provider'

type Draft = {
  hostID: string
  original: string
  value: string
  saving?: boolean
  failed?: boolean
}

export function useHostNotes(updateNotes: (id: string, notes: string) => Promise<void>) {
  const { t } = useTranslation('hosts')
  const { message } = useAppUI()
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const savingHosts = useRef(new Set<string>())
  const [closeRequest, setCloseRequest] = useState<{
    groupID: string
    resolve: (allowed: boolean) => void
  }>()
  const hasCloseRequest = useRef(false)

  function setDraft(groupID: string, draft: Draft) {
    setDrafts((current) => ({ ...current, [groupID]: draft }))
  }

  function discard(groupID: string) {
    setDrafts((current) => {
      if (!current[groupID]) return current
      const next = { ...current }
      delete next[groupID]
      return next
    })
  }

  async function save(groupID: string) {
    const draft = drafts[groupID]
    if (!draft || savingHosts.current.has(draft.hostID)) return false
    savingHosts.current.add(draft.hostID)
    setDraft(groupID, { ...draft, saving: true, failed: false })
    try {
      await updateNotes(draft.hostID, draft.value)
      discard(groupID)
      void message.success(t('notesSaved'))
      return true
    } catch {
      setDraft(groupID, { ...draft, saving: false, failed: true })
      return false
    } finally {
      savingHosts.current.delete(draft.hostID)
    }
  }

  function confirmClose(groupID: string): Promise<boolean> {
    const draft = drafts[groupID]
    if (!draft) return Promise.resolve(true)
    if (draft.saving || savingHosts.current.has(draft.hostID)) return Promise.resolve(false)
    if (draft.original === draft.value) return Promise.resolve(true)
    if (hasCloseRequest.current) return Promise.resolve(false)
    hasCloseRequest.current = true
    return new Promise((resolve) => setCloseRequest({ groupID, resolve }))
  }

  function finishClose(allowed: boolean) {
    hasCloseRequest.current = false
    setCloseRequest(undefined)
    closeRequest?.resolve(allowed)
  }

  const closingDraft = closeRequest && drafts[closeRequest.groupID]
  const closeDialog = closeRequest && (
    <ConfirmationDialog
      title={t('unsavedNotesTitle')}
      content={
        <>
          {t('unsavedNotesDescription')}
          {closingDraft?.failed && (
            <p role="alert" className="mt-2 text-destructive">{t('notesSaveFailed')}</p>
          )}
        </>
      }
      cancelText={t('continueEditingNotes')}
      okText={t('saveNotes')}
      pending={closingDraft?.saving}
      extraAction={{ label: t('discardNotesChanges'), onClick: () => finishClose(true) }}
      onCancel={() => finishClose(false)}
      onConfirm={async () => {
        if (await save(closeRequest.groupID)) finishClose(true)
      }}
    />
  )

  return {
    drafts,
    begin: (groupID: string, hostID: string, value: string) =>
      setDraft(groupID, { hostID, original: value, value }),
    change: (groupID: string, value: string) => {
      const draft = drafts[groupID]
      if (draft) setDraft(groupID, { ...draft, value, failed: false })
    },
    discard,
    save,
    confirmClose,
    closeDialog
  }
}
