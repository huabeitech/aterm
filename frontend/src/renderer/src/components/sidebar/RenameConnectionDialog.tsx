import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { t } from '../../../../shared/i18n'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export function RenameConnectionDialog({
  initialName,
  onSave,
  onClose
}: {
  initialName: string
  onSave: (name: string) => Promise<string | null>
  onClose: () => void
}) {
  useTranslation()
  const [name, setName] = useState(initialName)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  async function save() {
    if (saving || !name.trim()) return
    setSaving(true)
    try {
      const error = await onSave(name.trim())
      if (error) setError(error)
      else onClose()
    } finally {
      setSaving(false)
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saving) onClose()
      }}
    >
      <DialogContent
        className="sm:max-w-sm"
        aria-describedby={undefined}
        showCloseButton={!saving}
        onEscapeKeyDown={(e) => {
          if (saving) e.preventDefault()
        }}
        onPointerDownOutside={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t('hosts:rename')}</DialogTitle>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void save()
          }}
          className="space-y-4"
        >
          <Input
            autoFocus
            aria-label={t('hosts:hostDisplayName')}
            value={name}
            disabled={saving}
            aria-invalid={!!error}
            onFocus={(e) => e.target.select()}
            onChange={(e) => {
              setName(e.target.value)
              setError(null)
            }}
          />
          {error && (
            <p role="alert" className="text-xs text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" disabled={saving} onClick={onClose}>
              {t('hosts:cancel')}
            </Button>
            <Button type="submit" disabled={saving || !name.trim()}>
              {t('hosts:save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
