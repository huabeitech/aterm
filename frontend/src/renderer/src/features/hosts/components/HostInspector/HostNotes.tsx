import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, Pencil, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

export function HostNotes({
  notes,
  draft,
  onEdit,
  onChange,
  onCancel,
  onSave
}: {
  notes: string
  draft?: { original: string; value: string; saving?: boolean; failed?: boolean }
  onEdit: () => void
  onChange: (value: string) => void
  onCancel: () => void
  onSave: () => Promise<boolean>
}) {
  const { t } = useTranslation('hosts')
  const headingID = useId()
  const textID = useId()
  const errorID = useId()
  const textarea = useRef<HTMLTextAreaElement>(null)
  const preview = useRef<HTMLParagraphElement>(null)
  const editButton = useRef<HTMLButtonElement>(null)
  const restoreFocus = useRef(false)
  const [expanded, setExpanded] = useState(false)
  const [overflows, setOverflows] = useState(false)
  const editing = !!draft
  const canSave = draft && draft.value !== draft.original && !draft.saving

  useEffect(() => {
    if (editing) textarea.current?.focus()
    else if (restoreFocus.current) editButton.current?.focus()
    restoreFocus.current = false
  }, [editing])

  useLayoutEffect(() => {
    const element = preview.current
    if (!element || expanded) return
    const measure = () => setOverflows(element.scrollHeight > element.clientHeight + 1)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [editing, expanded, notes])

  async function save() {
    if (!canSave) return
    restoreFocus.current = true
    if (!(await onSave())) restoreFocus.current = false
  }

  return (
    <section aria-labelledby={headingID} className="min-w-0">
      <div className="mb-2 flex min-h-6 flex-wrap items-center gap-x-2">
        <h3 id={headingID} className="m-0 font-semibold text-[var(--mac-text)]">{t('notes')}</h3>
        {!draft && (
          <Button
            ref={editButton}
            type="button"
            size="xs"
            variant="ghost"
            aria-label={notes ? t('editNotes') : undefined}
            onClick={onEdit}
          >
            {notes ? <Pencil aria-hidden="true" /> : <Plus aria-hidden="true" />}
            {notes ? t('edit') : t('addNotes')}
          </Button>
        )}
      </div>
      {draft ? (
        <form
          className="grid gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
          onKeyDown={(event) => {
            if (
              event.key.toLowerCase() === 's' &&
              (event.metaKey || event.ctrlKey) &&
              !event.nativeEvent.isComposing
            ) {
              event.preventDefault()
              event.stopPropagation()
              void save()
            }
          }}
        >
          <Textarea
            ref={textarea}
            aria-labelledby={headingID}
            aria-describedby={draft.failed ? errorID : undefined}
            className="min-h-24 max-h-[60vh] field-sizing-fixed resize-y bg-[var(--mac-content)] text-[length:var(--app-text-13)] md:text-[length:var(--app-text-13)]"
            rows={4}
            value={draft.value}
            readOnly={draft.saving}
            onChange={(event) => onChange(event.target.value)}
          />
          {draft.failed && (
            <p id={errorID} role="alert" className="text-destructive">{t('notesSaveFailed')}</p>
          )}
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={draft.saving}
              onClick={() => {
                restoreFocus.current = true
                onCancel()
              }}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" size="sm" disabled={!canSave}>
              {draft.saving && <Loader2 className="animate-spin" aria-hidden="true" />}
              {t('saveNotes')}
            </Button>
          </div>
        </form>
      ) : (
        <div className="rounded-md bg-[var(--mac-content)] py-2">
          {notes ? (
            <>
              <p
                id={textID}
                ref={preview}
                className={cn('m-0 whitespace-pre-wrap wrap-anywhere leading-relaxed', !expanded && 'line-clamp-4')}
              >
                {notes}
              </p>
              {(overflows || expanded) && (
                <Button
                  type="button"
                  size="xs"
                  variant="link"
                  className="mt-1 px-0"
                  aria-controls={textID}
                  aria-expanded={expanded}
                  onClick={() => setExpanded(!expanded)}
                >
                  {t(expanded ? 'collapseNotes' : 'expandNotes')}
                </Button>
              )}
            </>
          ) : (
            <p className="m-0 text-[var(--mac-secondary-text)]">{t('noNotes')}</p>
          )}
        </div>
      )}
    </section>
  )
}
