import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ActionButton, WorkbenchDialog } from '@/components/ui/workbench-controls'
import type { UploadConflict } from '../../../../shared/contracts/files'
import { fileSize } from './fileFormat'

export function UploadConflictDialog({
  conflict,
  onClose,
  onChoose
}: {
  conflict: UploadConflict
  onClose: () => void
  onChoose: (action: 'skip' | 'overwrite', all: boolean) => Promise<void>
}) {
  const { t, i18n } = useTranslation('files')
  const [all, setAll] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const skip = useRef<HTMLButtonElement>(null)
  const regular = conflict.localKind === 'file' && conflict.remoteKind === 'file'
  const formatTime = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: 'medium',
    timeStyle: 'short'
  })
  async function choose(action: 'skip' | 'overwrite') {
    setBusy(true)
    try {
      await onChoose(action, all)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('transferOperationFailed'))
      setBusy(false)
    }
  }
  return (
    <WorkbenchDialog
      open
      title={t('conflictTitle', { name: conflict.path.split('/').pop() })}
      onOpened={(event) => {
        event.preventDefault()
        skip.current?.focus()
      }}
      onClose={onClose}
      busy={busy}
      actions={
        <>
          <ActionButton ref={skip} disabled={busy} onClick={() => void choose('skip')}>
            {t(all ? 'skipAll' : 'skipFile')}
          </ActionButton>
          {regular && (
            <ActionButton disabled={busy} onClick={() => void choose('overwrite')}>
              {t(all ? 'overwriteAll' : 'overwriteFile')}
            </ActionButton>
          )}
        </>
      }
    >
      <p className="break-all select-text">{conflict.path}</p>
      <table className="w-full text-left text-sm">
        <thead>
          <tr>
            <th />
            <th>{t('type')}</th>
            <th>{t('size')}</th>
            <th>{t('modified')}</th>
          </tr>
        </thead>
        <tbody>
          {(['local', 'remote'] as const).map((side) => {
            const kind = conflict[`${side}Kind`]
            return (
              <tr key={side}>
                <th>{t(side)}</th>
                <td>{t(kind === 'link' ? 'symbolicLink' : kind)}</td>
                <td>{kind === 'file' ? fileSize(conflict[`${side}Size`]) : '—'}</td>
                <td>{formatTime.format(new Date(conflict[`${side}Time`]))}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
      {regular && conflict.multiple && (
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            disabled={busy}
            checked={all}
            onChange={(e) => setAll(e.target.checked)}
          />
          {t('batchChoice')}
        </label>
      )}
      {error && <p role="alert">{error}</p>}
    </WorkbenchDialog>
  )
}
