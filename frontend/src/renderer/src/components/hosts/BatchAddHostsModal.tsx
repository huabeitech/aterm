import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { TableHeader, TableBody, TableRow, TableCell, TableHead } from '@/components/ui/table'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter
} from '@/components/ui/dialog'
import { useAppUI } from '@/components/ui/confirm-provider'
import { ClipboardList, Plus } from 'lucide-react'
import { ImportHostSnippet, PreviewHostSnippetImport } from '../../api/hosts'
import { domain as service } from '../../api/types'

const snippetPlaceholder = `Host prod-api
    User root
    HostName 192.168.1.10
    Port 22
    IdentityFile ~/.ssh/id_ed25519

Host staging
    User ubuntu
    HostName staging.example.com`

type BatchAddHostsModalProps = {
  open: boolean
  onCancel: () => void
  onImported: (firstCreatedID: string | null, createdCount: number) => Promise<void>
}

export function BatchAddHostsModal({ open, onCancel, onImported }: BatchAddHostsModalProps) {
  useTranslation()
  const { notification } = useAppUI()
  const [content, setContent] = useState('')
  const [preview, setPreview] = useState<service.HostSnippetPreview | null>(null)
  const [selectedRowIDs, setSelectedRowIDs] = useState<string[]>([])
  const [previewing, setPreviewing] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (open) return
    setContent('')
    setPreview(null)
    setSelectedRowIDs([])
    setPreviewing(false)
    setSubmitting(false)
  }, [open])

  const selectableRows = preview?.rows?.filter((row) => row.selectable) ?? []
  const canPreview = content.trim().length > 0 && !previewing && !submitting
  const canSubmit = selectedRowIDs.length > 0 && !previewing && !submitting

  async function previewContent() {
    if (!content.trim()) return
    setPreviewing(true)
    try {
      const nextPreview = await PreviewHostSnippetImport(content)
      setPreview(nextPreview)
      setSelectedRowIDs(
        (nextPreview.rows ?? []).filter((row) => row.selectable).map((row) => row.rowID)
      )
    } catch (error) {
      notification.error({
        message: t('hosts:unableToPreviewHosts'),
        description: errorText(error)
      })
    } finally {
      setPreviewing(false)
    }
  }

  async function importSelected() {
    if (!preview || selectedRowIDs.length === 0) return
    setSubmitting(true)
    try {
      const result = await ImportHostSnippet({ content: preview.content, rowIDs: selectedRowIDs })
      await onImported(result.created?.[0]?.id ?? null, result.created?.length ?? 0)
      onCancel()
    } catch (error) {
      notification.error({
        message: t('hosts:unableToAddHosts'),
        description: errorText(error)
      })
    } finally {
      setSubmitting(false)
    }
  }

  function updateContent(value: string) {
    setContent(value)
    setPreview(null)
    setSelectedRowIDs([])
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value && !submitting) onCancel()
      }}
    >
      <DialogContent
        className="batch-add-dialog flex max-h-[85vh] flex-col gap-4 sm:max-w-[800px]"
        aria-describedby={undefined}
        showCloseButton={!submitting}
        onPointerDownOutside={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => {
          if (submitting) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>{t('hosts:batchAddHosts')}</DialogTitle>
        </DialogHeader>
        <div className="batch-add-modal grid gap-3 [&_textarea]:font-mono [&_textarea]:text-xs [&_textarea]:leading-[calc(1.125rem*var(--app-font-scale))] min-h-0 space-y-3 overflow-y-auto">
          <p className="text-xs text-muted-foreground">
            {t('hosts:pasteOneOrMoreSshHostBlocksCommentsAndBlankLinesAreIgnored')}
          </p>
          <Textarea
            aria-label={t('hosts:sshConfiguration')}
            className="min-h-48 font-mono text-xs"
            value={content}
            onChange={(event) => updateContent(event.target.value)}
            placeholder={snippetPlaceholder}
            rows={9}
            spellCheck={false}
          />
          <details className="text-xs text-muted-foreground">
            <summary className="cursor-pointer py-1">{t('hosts:supportedFormat')}</summary>
            <ul className="list-disc space-y-1 py-2 pl-5">
              {(
                [
                  'snippetHostNameRequired',
                  'snippetComments',
                  'existingAliasesAreSkipped',
                  'snippetWildcards',
                  'snippetAliases'
                ] as const
              ).map((key) => (
                <li key={key}>{t(`hosts:${key}`)}</li>
              ))}
            </ul>
          </details>
          {preview && (
            <>
              <div className="max-h-60 overflow-auto rounded-md border border-border">
                <table className="w-full min-w-[720px] text-left text-xs">
                  <TableHeader className="sticky top-0 bg-background">
                    <TableRow>
                      <TableHead className="p-2">
                        <Checkbox
                          aria-label={t('hosts:selectAllHosts')}
                          checked={
                            selectableRows.length > 0 &&
                            selectedRowIDs.length === selectableRows.length
                              ? true
                              : selectedRowIDs.length
                                ? 'indeterminate'
                                : false
                          }
                          disabled={!selectableRows.length}
                          onCheckedChange={(checked) =>
                            setSelectedRowIDs(checked ? selectableRows.map((row) => row.rowID) : [])
                          }
                        />
                      </TableHead>
                      {[
                        t('hosts:hostAlias'),
                        t('hosts:address'),
                        t('hosts:username'),
                        t('hosts:port'),
                        t('hosts:privateKeyFile'),
                        t('hosts:status')
                      ].map((label) => (
                        <TableHead key={label} className="p-2 font-medium">
                          {label}
                        </TableHead>
                      ))}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(preview.rows ?? []).map((row) => (
                      <TableRow key={row.rowID} className="border-t border-border">
                        <TableCell className="p-2">
                          <Checkbox
                            aria-label={t('hosts:selectHost', { name: row.alias })}
                            disabled={!row.selectable}
                            checked={selectedRowIDs.includes(row.rowID)}
                            onCheckedChange={(checked) =>
                              setSelectedRowIDs((ids) =>
                                checked ? [...ids, row.rowID] : ids.filter((id) => id !== row.rowID)
                              )
                            }
                          />
                        </TableCell>
                        {[row.alias, row.hostName, row.user, row.port, row.identityFile].map(
                          (value, index) => (
                            <TableCell key={index} className="p-2">
                              {value || '-'}
                            </TableCell>
                          )
                        )}
                        <TableCell className="p-2">
                          <span
                            className={
                              row.selectable
                                ? 'text-emerald-600 dark:text-emerald-400'
                                : 'text-muted-foreground'
                            }
                          >
                            {t(`native:${row.statusKey}`)}
                          </span>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </table>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('hosts:readyHosts', { count: selectableRows.length })}
              </p>
            </>
          )}
        </div>
        <DialogFooter className="shrink-0">
          <Button variant="outline" onClick={onCancel} disabled={submitting}>
            {t('hosts:cancel')}
          </Button>
          <Button variant="outline" onClick={previewContent} disabled={!canPreview}>
            <ClipboardList size={15} />
            {previewing ? t('hosts:previewing') : t('hosts:preview')}
          </Button>
          <Button onClick={importSelected} disabled={!canSubmit}>
            <Plus size={15} />
            {submitting ? t('hosts:adding') : t('hosts:addHosts')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function errorText(error: unknown) {
  if (error instanceof Error) return error.message
  return String(error || t('hosts:unknownError'))
}
