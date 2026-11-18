import {
  encryptionPasswordCopy,
  encryptionPasswordError,
  type EncryptionPasswordValidation
} from '../../../../shared/security/passwordPrompts'
import { t } from '../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { PasswordInput } from '@/components/ui/desktop'
import { useAppUI } from '@/components/ui/confirm-provider'
import { useEffect, useState } from 'react'

import { desktop } from '../../api/app'
import type { IntegrationStatus } from '../../../../shared/contracts/integration'
export function useIntegration(unlock: () => Promise<boolean>, afterRestore: () => Promise<void>) {
  const { i18n } = useTranslation()
  const { modal, message } = useAppUI()
  const [status, setStatus] = useState<IntegrationStatus | null>(null),
    [busy, setBusy] = useState(false)
  async function refresh() {
    setStatus(await desktop.integration.status())
  }
  useEffect(() => {
    void refresh().catch((error) => message.error(error.message))
  }, [i18n.language])
  async function run(
    operation: () => Promise<IntegrationStatus>,
    secure = false,
    throwOnError = false
  ) {
    if (busy) return
    try {
      if (secure && !(await unlock())) return
      setBusy(true)
      const result = await operation()
      setStatus(result)
      return result
    } catch (error) {
      void message.error(error instanceof Error ? error.message : t('security:operationFailed'))
      if (throwOnError) throw error
    } finally {
      setBusy(false)
    }
  }
  function passwordDialog(kind: 'restore' | 'change', file = false) {
    let password = '',
      next = '',
      confirmation = ''
    let validationKey: EncryptionPasswordValidation | '' = ''
    let failure: unknown = null
    let closed = false
    let dialog: ReturnType<typeof modal.confirm>
    const close = () => {
      closed = true
      i18n.off('languageChanged', updateDialog)
      password = next = confirmation = ''
    }
    const renderContent = () => (
      <div className="sync-password-dialog">
        <p className="secure-data-description">{encryptionPasswordCopy(kind).description}</p>
        <div className="sync-password-dialog-fields grid gap-3 [&_label]:grid [&_label]:gap-1.5 [&_span]:text-[rgba(60,_60,_67,_0.78)] [&_span]:text-xs [&_span]:font-semibold [&_span]:leading-[calc(1rem*var(--app-font-scale))]">
          <label>
            <span>
              {t(kind === 'restore' ? 'security:encryptionPassword' : 'security:currentEncryptionPassword')}
            </span>
            <PasswordInput
              autoFocus
              autoComplete="current-password"
              defaultValue={password}
              onChange={(e) => {
                password = e.target.value
              }}
            />
          </label>
          {kind === 'change' && (
            <>
              <label>
                <span>{t('security:newEncryptionPassword')}</span>
                <PasswordInput
                  autoComplete="new-password"
                  defaultValue={next}
                  onChange={(e) => {
                    next = e.target.value
                  }}
                />
              </label>
              <label>
                <span>{t('security:confirmNewEncryptionPassword')}</span>
                <PasswordInput
                  autoComplete="new-password"
                  defaultValue={confirmation}
                  onChange={(e) => {
                    confirmation = e.target.value
                  }}
                />
              </label>
            </>
          )}
        </div>
        {(validationKey || !!failure) && (
          <div className="sync-dialog-error" role="alert">
            {validationKey ? t(`security:${validationKey}`) : encryptionPasswordError(failure)}
          </div>
        )}
      </div>
    )
    const updateDialog = () => {
      if (!closed) dialog.update({ ...encryptionPasswordCopy(kind), content: renderContent() })
    }
    const rejectValidation = (key: EncryptionPasswordValidation) => {
      validationKey = key
      updateDialog()
      throw Error(t(`security:${key}`))
    }
    dialog = modal.confirm({
      ...encryptionPasswordCopy(kind),
      submitOnEnter: kind === 'restore',
      content: renderContent(),
      onCancel: close,
      onOk: async () => {
        validationKey = ''
        failure = null
        if (!password.trim()) rejectValidation('enterEncryptionPassword')
        if (kind === 'change' && next.length < 8) rejectValidation('encryptionPasswordTooShort')
        if (kind === 'change' && next !== confirmation)
          rejectValidation('newEncryptionPasswordsDoNotMatch')
        updateDialog()
        try {
          const previous = await desktop.integration.status()
          const result =
            kind === 'restore'
              ? await desktop.integration.restore(password, file)
              : await desktop.integration.changePassword(password, next)
          setStatus(result)
          if (kind === 'restore' && result.sync.lastDownload !== previous.sync.lastDownload)
            await afterRestore()
          close()
        } catch (error) {
          failure = error
          updateDialog()
          throw error
        }
      }
    })
    i18n.on('languageChanged', updateDialog)
  }

  return {
    status,
    busy,
    refresh,
    choose: (kind: 'ssh' | 'sync' | 'icloud') =>
      void run(() => desktop.integration.chooseDirectory(kind)),
    backup: (file = false) => {
      const backup = () =>
        run(async () => {
          const previous = await desktop.integration.status()
          const result = await desktop.integration.backup(file)
          if (!file && result.sync.lastUpload !== previous.sync.lastUpload)
            void message.success(t('security:icloudBackupComplete'))
          return result
        }, true)
      if (file) {
        void backup()
        return
      }
      modal.confirm({
        title: t('security:backUpToIcloud'),
        okText: t('security:backUp'),
        content: t('security:replaceBackupWarning'),
        onOk: async () => {
          await run(
            async () => {
              const previous = await desktop.integration.status()
              const result = await desktop.integration.backup(false)
              if (result.sync.lastUpload !== previous.sync.lastUpload)
                void message.success(t('security:icloudBackupComplete'))
              return result
            },
            true,
            true
          )
        }
      })
    },
    restore: (file = false) => passwordDialog('restore', file),
    changePassword: () => passwordDialog('change'),
    autoUnlock: (enable = true) => void run(() => desktop.integration.autoUnlock(enable), enable),
    exportSSH: () =>
      modal.confirm({
        title: t('security:exportOpensshConfiguration'),
        okText: t('security:export'),
        content: (
          <>
            {t('security:exportWarning', {
              generatedPath:
                status?.openssh.generatedConfigPath ?? '~/.ssh/config.d/ssh-connect.generated.conf',
              configPath: status?.openssh.sshConfigPath ?? '~/.ssh/config'
            })}
          </>
        ),
        onOk: async () => {
          await run(
            async () => {
              const previous = await desktop.integration.status()
              const result = await desktop.integration.exportSSH()
              if (result.openssh.lastExportedAt !== previous.openssh.lastExportedAt)
                void message.success(t('security:exportComplete'))
              return result
            },
            false,
            true
          )
        }
      }),
    deleteBackup: () => void run(() => desktop.integration.deleteBackup())
  }
}
