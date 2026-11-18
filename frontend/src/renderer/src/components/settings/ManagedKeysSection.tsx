import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useEffect, useState } from 'react'
import { Copy, FileKey, LockKeyhole, Send, Trash2 } from 'lucide-react'
import { desktop } from '../../api/app'
import type { domain } from '../../api/types'
import { ActionButton, TextField } from '../ui/workbench-controls'
import { Button } from '../ui/button'
import { ConfirmationDialog } from '../ui/confirmation-dialog'
import { useAppUI } from '../ui/confirm-provider'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '../ui/dialog'
import { toast } from '../ui/toast'
import { Tip } from '../ui/desktop'

type Draft = {
  name: string
  algorithm: domain.ManagedKey['algorithm']
  passphrase: string
  confirmation: string
}
const blank = (): Draft => ({ name: '', algorithm: 'ed25519', passphrase: '', confirmation: '' })
export function ManagedKeysSection({
  macAppStoreBuild,
  unlocked
}: {
  macAppStoreBuild: boolean
  unlocked: boolean
}) {
  useTranslation()
  const { modal } = useAppUI()
  const [keys, setKeys] = useState<domain.ManagedKey[]>([]),
    [busy, setBusy] = useState(false)
  const [createOpen, setCreateOpen] = useState(false),
    [importOpen, setImportOpen] = useState(false),
    [draft, setDraft] = useState<Draft>(blank())
  const [selected, setSelected] = useState<domain.ManagedKeyImportPreview | null>(null),
    [formError, setFormError] = useState('')
  const [blockedReferences, setBlockedReferences] = useState<domain.Host[] | null>(null)
  const [exportKey, setExportKey] = useState<domain.ManagedKey | null>(null)
  const [agentKey, setAgentKey] = useState<domain.ManagedKey | null>(null)
  const [agentPassphrase, setAgentPassphrase] = useState('')
  const load = () =>
    desktop.library
      .listManagedKeys()
      .then(setKeys)
      .catch((value) =>
        toast.add({
          title: t('settings:unableToLoadSshKeys'),
          description: value.message,
          type: 'error'
        })
      )
  useEffect(() => {
    if (unlocked) void load()
    else setKeys([])
  }, [unlocked])
  const run = async (
    action: () => Promise<unknown>,
    close: (() => void) | undefined,
    success: string
  ) => {
    try {
      setBusy(true)
      await action()
      await load()
      close?.()
      toast.add({ title: success, type: 'success' })
    } catch (value) {
      toast.add({
        title: t('settings:sshKeyActionFailed'),
        description: value instanceof Error ? value.message : t('settings:unableToUpdateSshKeys'),
        type: 'error'
      })
    } finally {
      setBusy(false)
    }
  }
  const create = () => {
    if (draft.passphrase !== draft.confirmation) {
      setFormError(t('settings:thePrivateKeyPassphrasesDoNotMatch'))
      return
    }
    setFormError('')
    void run(
      async () => {
        await desktop.library.createManagedKey({
          name: draft.name,
          algorithm: draft.algorithm,
          passphrase: draft.passphrase
        })
      },
      () => {
        setCreateOpen(false)
        setDraft(blank())
      },
      t('settings:sshKeyGenerated')
    )
  }
  const choose = async () => {
    try {
      setBusy(true)
      const preview = await desktop.library.chooseManagedKeyImport()
      if (preview) {
        setSelected(preview)
        setDraft({
          ...blank(),
          name: preview.suggestedName,
          algorithm: preview.algorithm || 'ed25519'
        })
        setImportOpen(true)
      }
    } catch (value) {
      toast.add({
        title: t('settings:unableToReadPrivateKey'),
        description: value instanceof Error ? value.message : undefined,
        type: 'error'
      })
    } finally {
      setBusy(false)
    }
  }
  const imported = () =>
    run(
      async () => {
        if (!selected) throw Error(t('settings:chooseAPrivateKeyFirst'))
        await desktop.library.importManagedKey(selected.token, draft.name, draft.passphrase)
      },
      () => {
        setImportOpen(false)
        setSelected(null)
        setDraft(blank())
      },
      t('settings:sshKeyImported')
    )
  const remove = async (key: domain.ManagedKey) => {
    try {
      setBusy(true)
      const hosts = await desktop.library.listHosts()
      const references = hosts.filter((host) => host.identityFile === `aterm-key://${key.id}`)
      if (references.length) {
        setBlockedReferences(references)
        return
      }
      modal.confirm({
        title: t('settings:deleteKeyConfirm', { name: key.name }),
        okText: t('settings:deleteSshKey'),
        okButtonProps: { danger: true },
        onOk: () =>
          run(
            () => desktop.library.deleteManagedKey(key.id),
            undefined,
            t('settings:sshKeyDeleted')
          )
      })
    } catch (value) {
      toast.add({
        title: t('settings:unableToCheckSshKeyUsage'),
        description: value instanceof Error ? value.message : undefined,
        type: 'error'
      })
    } finally {
      setBusy(false)
    }
  }
  const exportPrivateKey = async () => {
    if (!exportKey) return
    try {
      setBusy(true)
      if (await desktop.library.exportManagedKey(exportKey.id)) {
        setExportKey(null)
        toast.add({
          title: t('settings:privateKeyExportedWithOwnerOnlyFilePermissions'),
          type: 'success'
        })
      }
    } catch (value) {
      toast.add({
        title: t('settings:sshKeyActionFailed'),
        description: value instanceof Error ? value.message : t('settings:unableToExportSshKey'),
        type: 'error'
      })
    } finally {
      setBusy(false)
    }
  }
  const addToAgent = () => {
    if (!agentKey) return
    const passphrase = agentPassphrase
    setAgentPassphrase('')
    void run(
      () => desktop.library.addManagedKeyToAgent(agentKey.id, passphrase),
      () => {
        setAgentKey(null)
        setAgentPassphrase('')
      },
      t('settings:sshKeyAddedToSshAgentOpensshCommandsCanNowUseIt')
    )
  }
  return (
    <section className="settings-keys" aria-label={t('settings:sshKeys')}>
      <div className="settings-keys-heading">
        <h2>{t('settings:sshKeys')}</h2>
        <div className="settings-key-actions">
          <ActionButton disabled={busy || !unlocked} onClick={() => void choose()}>
            {t('settings:import')}
          </ActionButton>
          <ActionButton disabled={busy || !unlocked} onClick={() => setCreateOpen(true)}>
            {t('settings:newSshKey')}
          </ActionButton>
        </div>
      </div>
      <div className="settings-keys-list">
        {keys.length === 0 ? (
          <div className="settings-key-empty">
            {unlocked
              ? t('settings:noSshKeysImportAnExistingKeyOrCreateOne')
              : t('security:lockedDescription')}
          </div>
        ) : (
          keys.map((key) => (
            <div key={key.id} className="settings-key-row">
              <div className="min-w-0">
                <div className="settings-key-name">{key.name}</div>
                <div className="settings-key-detail">
                  {key.algorithm === 'rsa'
                    ? 'RSA 4096'
                    : key.algorithm === 'ecdsa'
                      ? 'ECDSA P-256'
                      : 'Ed25519'}{' '}
                  · {key.fingerprint}
                </div>
                <div className="settings-key-detail">
                  {key.passphraseProtected
                    ? t('settings:privateKeyPassphraseRequired')
                    : t('settings:protectedByEncryptedVault')}
                </div>
              </div>
              <div className="flex gap-1">
                <Tip title={t('settings:copyPublicKey')}>
                  <ActionButton
                    aria-label={t('settings:copyPublicKey')}
                    icon={<Copy size={15} />}
                    disabled={busy || !unlocked}
                    onClick={() =>
                      void desktop.library
                        .copyText(key.publicKey)
                        .then(() =>
                          toast.add({ title: t('settings:publicKeyCopied'), type: 'success' })
                        )
                        .catch((value) =>
                          toast.add({
                            title: t('settings:unableToCopyPublicKey'),
                            description: value.message,
                            type: 'error'
                          })
                        )
                    }
                  />
                </Tip>
                {!macAppStoreBuild && (
                  <Tip title={t('settings:addToSshAgent')}>
                    <ActionButton
                      aria-label={t('settings:addKeyToAgent', { name: key.name })}
                      icon={<Send size={15} />}
                      disabled={busy || !unlocked}
                      onClick={() => {
                        setAgentPassphrase('')
                        setAgentKey(key)
                      }}
                    />
                  </Tip>
                )}
                <Tip title={t('settings:exportPrivateKeyForOpenssh')}>
                  <ActionButton
                    aria-label={t('settings:exportNamedKey', { name: key.name })}
                    icon={<FileKey size={15} />}
                    disabled={busy || !unlocked}
                    onClick={() => setExportKey(key)}
                  />
                </Tip>
                <Tip title={t('settings:deleteSshKey')}>
                  <ActionButton
                    aria-label={t('settings:deleteNamedKey', { name: key.name })}
                    icon={<Trash2 size={15} />}
                    disabled={busy || !unlocked}
                    onClick={() => void remove(key)}
                  />
                </Tip>
              </div>
            </div>
          ))
        )}
      </div>
      <p className="settings-footnote">{t('security:privateKeyHelp')}</p>
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('settings:newSshKey2')}</DialogTitle>
          </DialogHeader>
          <KeyForm draft={draft} setDraft={setDraft} algorithm error={formError} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              {t('settings:cancel')}
            </Button>
            <Button disabled={busy || !draft.name.trim()} onClick={() => void create()}>
              {busy ? t('settings:generating') : t('settings:generateKey')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={importOpen}
        onOpenChange={(open) => {
          setImportOpen(open)
          if (!open) setSelected(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('settings:importPrivateKey')}</DialogTitle>
          </DialogHeader>
          {selected ? (
            <>
              <div className="rounded-md bg-muted p-3 text-sm">
                <strong>{selected.sourceName}</strong>
                {selected.fingerprint && (
                  <div className="settings-key-detail">{selected.fingerprint}</div>
                )}
              </div>
              <KeyForm
                draft={draft}
                setDraft={setDraft}
                passphraseLabel={
                  selected.requiresPassphrase
                    ? t('security:privateKeyPassphrase')
                    : t('security:optionalPrivateKeyPassphrase')
                }
              />
            </>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setImportOpen(false)}>
              {t('settings:cancel')}
            </Button>
            {selected && (
              <Button disabled={busy || !draft.name.trim()} onClick={() => void imported()}>
                {busy ? t('settings:importing') : t('settings:importKey')}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {exportKey && (
        <ConfirmationDialog
          title={t('settings:exportPrivateKey')}
          content={
            <p className="text-muted-foreground">
              {t('settings:exportKeyWarning', { name: exportKey.name })}
            </p>
          }
          okText={t('settings:exportPrivateKey2')}
          cancelText={t('settings:cancel')}
          pending={busy}
          confirmDisabled={!unlocked}
          onCancel={() => setExportKey(null)}
          onConfirm={() => void exportPrivateKey()}
        />
      )}
      {!macAppStoreBuild && agentKey && (
        <ConfirmationDialog
          title={t('settings:addKeyToSshAgent')}
          content={
            <div className="space-y-4">
              <p className="text-muted-foreground">
                {t('settings:agentKeyWarning', { name: agentKey.name })}
              </p>
              {agentKey.passphraseProtected && (
                <label className="grid gap-1 text-sm font-medium">
                  {t('security:privateKeyPassphrase')}
                  <TextField
                    aria-label={t('security:privateKeyPassphrase')}
                    type="password"
                    autoFocus
                    value={agentPassphrase}
                    onChange={(event) => setAgentPassphrase(event.target.value)}
                  />
                </label>
              )}
            </div>
          }
          okText={t('settings:addToSshAgent')}
          cancelText={t('settings:cancel')}
          pending={busy}
          confirmDisabled={agentKey.passphraseProtected && !agentPassphrase}
          onConfirm={addToAgent}
          onCancel={() => {
            setAgentKey(null)
            setAgentPassphrase('')
          }}
        />
      )}
      <Dialog
        open={!!blockedReferences}
        onOpenChange={(open) => {
          if (!open) setBlockedReferences(null)
        }}
      >
        <DialogContent className="sm:max-w-[440px]" aria-describedby={undefined}>
          <DialogHeader>
            <DialogTitle>{t('settings:cannotDeleteSshKey')}</DialogTitle>
          </DialogHeader>
          {blockedReferences && <BlockedKeyDeletionMessage hosts={blockedReferences} />}
          <DialogFooter>
            <Button onClick={() => setBlockedReferences(null)}>{t('native:ok')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}
function BlockedKeyDeletionMessage({ hosts }: { hosts: domain.Host[] }) {
  useTranslation()
  const shown = hosts.slice(0, 3).map((host) => `“${host.alias}”`)
  return (
    <p className="text-[length:var(--app-text-13)] leading-relaxed text-muted-foreground">
      {t('settings:keyInUse', { hosts: shown.join(', '), count: hosts.length })}
    </p>
  )
}
function KeyForm({
  draft,
  setDraft,
  algorithm = false,
  passphraseLabel = t('security:optionalPrivateKeyPassphrase'),
  error = ''
}: {
  draft: Draft
  setDraft: (value: Draft) => void
  algorithm?: boolean
  passphraseLabel?: string
  error?: string
}) {
  useTranslation()
  const update = <K extends keyof Draft>(key: K, value: Draft[K]) =>
    setDraft({ ...draft, [key]: value })
  return (
    <div className="grid gap-3">
      <label className="grid gap-1 text-sm font-medium">
        <span>
          {t('settings:name')}{' '}
          <span className="text-destructive" aria-hidden="true">
            *
          </span>
        </span>
        <TextField
          aria-label={t('settings:sshKeyName')}
          value={draft.name}
          onChange={(e) => update('name', e.target.value)}
          placeholder={t('settings:eGProductionDeploy')}
        />
      </label>
      {algorithm && (
        <label className="grid gap-1 text-sm font-medium">
          <span>
            {t('settings:algorithm')}{' '}
            <span className="text-destructive" aria-hidden="true">
              *
            </span>
          </span>
          <select
            aria-label={t('settings:sshKeyAlgorithm')}
            className="h-9 rounded-md border bg-transparent px-3 text-sm"
            value={draft.algorithm}
            onChange={(e) => update('algorithm', e.target.value as Draft['algorithm'])}
          >
            <option value="ed25519">{t('settings:ed25519Recommended')}</option>
            <option value="rsa">{t('settings:rsaCompatibility')}</option>
            <option value="ecdsa">{t('settings:ecdsaP256RequiredEnvironments')}</option>
          </select>
        </label>
      )}
      <label className="grid gap-1 text-sm font-medium">
        {passphraseLabel}
        <TextField
          aria-label={t('security:privateKeyPassphrase')}
          type="password"
          value={draft.passphrase}
          onChange={(e) => update('passphrase', e.target.value)}
        />
      </label>
      {algorithm && (
        <label className="grid gap-1 text-sm font-medium">
          {t('security:confirmPrivateKeyPassphrase')}
          <TextField
            aria-label={t('security:confirmPrivateKeyPassphrase')}
            type="password"
            value={draft.confirmation}
            onChange={(e) => update('confirmation', e.target.value)}
          />
          {error && <span className="text-xs font-normal text-destructive">{error}</span>}
        </label>
      )}
      <p className="flex gap-1 text-xs text-muted-foreground">
        <LockKeyhole size={13} />
        {t('security:privateKeyHelp')}
      </p>
    </div>
  )
}
