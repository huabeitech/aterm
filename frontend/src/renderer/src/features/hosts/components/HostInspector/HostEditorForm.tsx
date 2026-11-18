import { t } from '../../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@/components/ui/collapsible'
import { useEffect, useState, type Ref } from 'react'
import { ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Field, PasswordInput, SearchSelect } from '@/components/ui/desktop'
import { domain } from '../../../../api/types'
import { desktop } from '../../../../api/app'
import type { BusyAction, FormState } from '../../../../components/common/types'
import { useSecureData } from '../../hooks/useSecureData'
import { useAppUI } from '@/components/ui/confirm-provider'
import { DetailSection } from './DetailSection'
import { RadioGroup, RadioGroupItem } from '../../../../components/ui/radio-group'

export function HostEditorForm({
  mode,
  form,
  directories,
  hosts,
  errors,
  aliasInputRef,
  onChange
}: {
  mode: 'create' | 'edit'
  form: FormState
  directories: { id: string; label: string }[]
  hosts: domain.Host[]
  errors: Partial<Record<keyof FormState, string>>
  aliasInputRef?: Ref<HTMLInputElement>
  onChange: (next: FormState) => void
}) {
  useTranslation()
  const update = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    onChange({ ...form, [key]: value })
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [managedKeys, setManagedKeys] = useState<domain.ManagedKey[]>([])
  const [keysLocked, setKeysLocked] = useState(true)
  const [keysBusy, setKeysBusy] = useState<BusyAction>(null)
  const { ensureSecureDataUnlocked } = useSecureData(setKeysBusy)
  const { message } = useAppUI()
  useEffect(() => {
    let active = true
    void (async () => {
      try {
        const status = await desktop.library.vaultStatus()
        if (!active) return
        setKeysLocked(!status.unlocked)
        if (status.unlocked) {
          const keys = await desktop.library.listManagedKeys()
          if (active) setManagedKeys(keys)
        }
      } catch {
        if (active) message.error(t('settings:unableToLoadSshKeys'))
      }
    })()
    return () => {
      active = false
    }
  }, [message])
  async function unlockManagedKeys() {
    try {
      if (!(await ensureSecureDataUnlocked())) return
      const keys = await desktop.library.listManagedKeys()
      setManagedKeys(keys)
      setKeysLocked(false)
    } catch {
      message.error(t('settings:unableToLoadSshKeys'))
    }
  }
  const advancedFields: (keyof FormState)[] = [
    'jumpHostID',
    'proxyJump',
    'forwardAgent',
    'compression',
    'serverAliveInterval',
    'serverAliveCountMax',
    'connectTimeout'
  ]
  const advancedError = advancedFields.some((key) => !!errors[key])
  useEffect(() => {
    if (advancedError) setAdvancedOpen(true)
  }, [advancedError])
  const passwordMode = form.passwordAuth
  const managedIdentity = form.keySource === 'managed'
  const legacyIdentity = !!form.identityFile && !form.identityFile.startsWith('aterm-key://')
  function selectAuthentication(value: string | number) {
    const usePassword = value === 'password'
    onChange({
      ...form,
      passwordAuth: usePassword,
      password: '',
      passwordState:
        usePassword && form.passwordState === 'saved'
          ? 'saved'
          : usePassword
            ? 'editing'
            : form.passwordState
    })
  }
  function removePassword() {
    onChange({
      ...form,
      passwordAuth: false,
      password: '',
      passwordState: 'none'
    })
  }
  return (
    <div className="host-editor grid gap-4.5 p-1">
      <DetailSection title={t('hosts:connection')}>
        <div className="host-editor-connection-grid grid grid-cols-[repeat(auto-fit,minmax(min(100%,16em),1fr))] gap-[10px_16px] max-[560px]:grid-cols-[minmax(0,_1fr)]">
          <Field
            label={t('hosts:hostAlias')}
            tooltip={t('hosts:hostAliasHelp')}
            required
            help={errors.alias}
          >
            <Input
              ref={aliasInputRef}
              autoFocus
              aria-required
              aria-label={t('hosts:hostAlias')}
              aria-invalid={!!errors.alias}
              value={form.alias}
              onChange={(event) => update('alias', event.target.value)}
              placeholder="prod-api"
              spellCheck={false}
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
            />
          </Field>
          <Field label={t('hosts:directory')}>
            <SearchSelect
              label={t('hosts:directory')}
              value={form.directoryID}
              onChange={(value) => update('directoryID', value)}
              options={[
                { value: '', label: t('hosts:rootDirectory') },
                ...directories.map((directory) => ({
                  value: directory.id,
                  label: directory.label
                }))
              ]}
            />
          </Field>
          <Field
            label={t('hosts:hostnameOrIpAddress')}
            tooltip={t('hosts:hostnameOrIpAddressOfTheRemoteHost')}
            required
            help={errors.hostName}
          >
            <Input
              aria-required
              aria-label={t('hosts:hostnameOrIpAddress')}
              aria-invalid={!!errors.hostName}
              value={form.hostName}
              onChange={(event) => update('hostName', event.target.value)}
              placeholder="10.12.4.18"
              spellCheck={false}
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
            />
          </Field>
          <Field label={t('hosts:port')} help={errors.port}>
            <Input
              type="number"
              min={1}
              max={65535}
              aria-label={t('hosts:port')}
              aria-invalid={!!errors.port}
              value={form.port}
              onChange={(event) => update('port', event.target.value)}
              className="full-width w-full"
            />
          </Field>
          <Field label={t('hosts:username')} className="host-editor-user [grid-column:1/-1]">
            <Input
              aria-label={t('hosts:username')}
              value={form.user}
              onChange={(event) => update('user', event.target.value)}
              placeholder="ubuntu"
              spellCheck={false}
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
            />
          </Field>
        </div>
      </DetailSection>
      <DetailSection title={t('hosts:authentication')}>
        <Tabs
          value={passwordMode ? 'password' : 'publicKey'}
          onValueChange={selectAuthentication}
          className="mb-3"
        >
          <TabsList className="h-8">
            <TabsTrigger value="publicKey">{t('hosts:publicKey')}</TabsTrigger>
            <TabsTrigger value="password">{t('hosts:password')}</TabsTrigger>
          </TabsList>
        </Tabs>
        {passwordMode ? (
          <>
            {mode === 'edit' && form.passwordState === 'saved' ? (
              <Field label="" extra={t('hosts:savedPasswordHelp')}>
                <div className="flex flex-wrap items-center gap-1">
                  <span>{t('hosts:passwordSaved')}</span>
                  <Button
                    variant="link"
                    size="sm"
                    onClick={() => update('passwordState', 'editing')}
                  >
                    {t('hosts:changePassword')}
                  </Button>
                  <Button
                    variant="link"
                    className="text-destructive"
                    size="sm"
                    onClick={removePassword}
                  >
                    {t('hosts:removeSavedPassword')}
                  </Button>
                </div>
              </Field>
            ) : (
              <Field label={t('hosts:password')} extra={t('hosts:storedInEncryptedSecureData')}>
                <PasswordInput
                  aria-label={t('hosts:password')}
                  value={form.password}
                  onChange={(event) => update('password', event.target.value)}
                  placeholder={t('hosts:enterPassword')}
                  autoComplete="new-password"
                />
              </Field>
            )}
          </>
        ) : (
          <div className="grid gap-4">
            <Field
              label={t('hosts:keySource')}
              // extra="Choose a key managed by aTerm, or let SSH use your system default keys and ssh-agent."
            >
              <RadioGroup
                aria-label={t('hosts:sshKeySource')}
                value={managedIdentity ? 'managed' : 'system'}
                className="grid-cols-[repeat(auto-fit,minmax(min(100%,16em),1fr))]"
                onValueChange={(value) => {
                  if (value === 'system')
                    onChange({ ...form, keySource: 'system', identityFile: '' })
                  else onChange({ ...form, keySource: 'managed', identityFile: '' })
                }}
              >
                <label
                  className={`flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors hover:bg-accent/50 ${!managedIdentity ? 'border-primary bg-primary/5' : 'border-border'}`}
                >
                  <RadioGroupItem value="system" />
                  <span className="grid gap-0.5">
                    <span className="text-sm font-medium">{t('hosts:systemDefaultKey')}</span>
                    <span className="text-xs text-muted-foreground">
                      {t('hosts:useSshKeysAndSshAgent')}
                    </span>
                  </span>
                </label>
                <label
                  className={`flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors hover:bg-accent/50 ${managedIdentity ? 'border-primary bg-primary/5' : 'border-border'}`}
                >
                  <RadioGroupItem value="managed" />
                  <span className="grid gap-0.5">
                    <span className="text-sm font-medium">{t('hosts:managedSshKey')}</span>
                    <span className="text-xs text-muted-foreground">
                      {t('hosts:chooseAKeyEncryptedByAterm')}
                    </span>
                  </span>
                </label>
              </RadioGroup>
            </Field>
            {legacyIdentity && (
              <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-800 dark:text-amber-300">
                {t('hosts:legacyKeyMigrationHelp')}
              </p>
            )}
            {managedIdentity && (
              <Field
                label={t('hosts:managedSshKey')}
                required
                help={errors.identityFile}
                extra={t('hosts:privateMaterialStaysEncryptedInAterm')}
              >
                {keysLocked ? (
                  <Button size="sm" disabled={!!keysBusy} onClick={() => void unlockManagedKeys()}>
                    {t('security:unlockAction')}
                  </Button>
                ) : (
                  <SearchSelect
                    label={t('hosts:managedSshKey')}
                    value={form.identityFile}
                    placeholder={t('hosts:selectAManagedKey')}
                    options={managedKeys.map((key) => ({
                      value: `aterm-key://${key.id}`,
                      label: `${key.name} · Ed25519`
                    }))}
                    onChange={(value) => update('identityFile', value)}
                  />
                )}
              </Field>
            )}
          </div>
        )}
      </DetailSection>
      <DetailSection title={t('hosts:metadata')}>
        <Field label={t('hosts:notes')}>
          <Textarea
            aria-label={t('hosts:notes')}
            value={form.remark}
            onChange={(event) => update('remark', event.target.value)}
            placeholder={t('hosts:primaryApiGateway')}
            rows={3}
          />
        </Field>
      </DetailSection>
      <Collapsible
        open={advancedOpen}
        onOpenChange={setAdvancedOpen}
        className="host-editor-advanced overflow-hidden rounded-md border bg-background"
      >
        <CollapsibleTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-9 w-full justify-start rounded-none px-3 font-normal text-muted-foreground hover:text-foreground"
          >
            <ChevronRight
              size={15}
              className="host-editor-disclosure shrink-0 text-[var(--mac-secondary-text)] transition-transform [.host-editor-advanced[data-state=open]_&]:rotate-90"
            />
            <span className="host-editor-advanced-label font-medium">
              {t('hosts:advancedSettings')}
            </span>
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="host-editor-advanced-content border-t px-3 pb-3 [.host-editor-advanced_&]:grid [.host-editor-advanced_&]:gap-4 [.host-editor-advanced_&]:pt-3">
          <DetailSection title={t('hosts:connectionOptions')}>
            <Field
              label={t('hosts:jumpHost')}
              extra={t('hosts:chooseAHostSavedInAtermLeaveEmptyForADirectConnection')}
            >
              <SearchSelect
                label={t('hosts:jumpHost')}
                value={form.jumpHostID || undefined}
                clearable
                placeholder={t('hosts:directConnection')}
                onChange={(value) => {
                  const jump = hosts.find((host) => host.id === value)
                  onChange({
                    ...form,
                    jumpHostID: value || '',
                    proxyJump: jump?.alias || ''
                  })
                }}
                options={hosts
                  .filter((host) => host.alias !== form.alias)
                  .map((host) => ({
                    value: host.id,
                    label: `${host.alias} — ${host.user ? `${host.user}@` : ''}${host.hostName}`
                  }))}
              />
            </Field>
            <div className="host-editor-switches grid gap-3 grid-cols-[repeat(auto-fit,minmax(min(100%,16em),1fr))]">
              <Field label={t('hosts:agentForwarding')}>
                <Switch
                  aria-label={t('hosts:agentForwarding')}
                  checked={form.forwardAgent}
                  onCheckedChange={(checked) => update('forwardAgent', checked)}
                />
              </Field>
              <Field label={t('hosts:compression')}>
                <Switch
                  aria-label={t('hosts:compression')}
                  checked={form.compression}
                  onCheckedChange={(checked) => update('compression', checked)}
                />
              </Field>
            </div>
          </DetailSection>
          <DetailSection title={t('hosts:reliability')}>
            <div className="host-editor-reliability-grid grid grid-cols-[repeat(auto-fit,minmax(min(100%,16em),1fr))] gap-3 max-[560px]:grid-cols-[minmax(0,_1fr)]">
              <Field
                label={t('hosts:keepaliveInterval')}
                tooltip={t('hosts:timeBetweenSshKeepaliveMessagesInSeconds')}
              >
                <Input
                  type="number"
                  min={0}
                  aria-label={t('hosts:keepaliveInterval')}
                  value={numberOrEmpty(form.serverAliveInterval)}
                  onChange={(event) => update('serverAliveInterval', event.target.value)}
                  className="full-width w-full"
                  placeholder={t('hosts:default')}
                />
              </Field>
              <Field
                label={t('hosts:maxUnansweredKeepalives')}
                tooltip={t('hosts:keepaliveCountHelp')}
              >
                <Input
                  type="number"
                  min={0}
                  aria-label={t('hosts:maxUnansweredKeepalives')}
                  value={numberOrEmpty(form.serverAliveCountMax)}
                  onChange={(event) => update('serverAliveCountMax', event.target.value)}
                  className="full-width w-full"
                  placeholder={t('hosts:default')}
                />
              </Field>
              <Field
                label={t('hosts:connectionTimeout')}
                tooltip={t('hosts:maximumTimeToEstablishTheSshConnectionInSeconds')}
              >
                <Input
                  type="number"
                  min={0}
                  aria-label={t('hosts:connectionTimeout')}
                  value={numberOrEmpty(form.connectTimeout)}
                  onChange={(event) => update('connectTimeout', event.target.value)}
                  className="full-width w-full"
                  placeholder={t('hosts:default')}
                />
              </Field>
            </div>
          </DetailSection>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}

function numberOrEmpty(value: string) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && value !== '' ? parsed : ''
}
