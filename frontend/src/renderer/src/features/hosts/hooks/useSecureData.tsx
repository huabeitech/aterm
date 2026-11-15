import { t } from '../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useId } from 'react'
import { PasswordInput } from '@/components/ui/desktop'
import { useAppUI } from '@/components/ui/confirm-provider'
import type { domain } from '../../../api/types'
import { desktop } from '../../../api/app'
import './secure-data-dialog.css'
export function useSecureData(
  setBusyAction: (value: import('../../../components/common/types').BusyAction) => void
) {
  useTranslation()
  const { modal } = useAppUI()
  const id = useId()
  const options = { modal, setBusyAction }
  const loadSyncStatus = () => desktop.library.vaultStatus()
  const SetUpSecureData = (password: string) => desktop.library.unlockVault(password, true)
  const UnlockSecureData = (password: string) => desktop.library.unlockVault(password, false)
  async function ensureSecureDataUnlocked(
    reason: 'save-password' | 'connect' | 'reveal-password' | 'sync'
  ) {
    const status = await loadSyncStatus()
    if (status.unlocked) return true
    return promptSecureDataUnlock(status, reason)
  }
  function promptSecureDataUnlock(
    status: Pick<domain.EncryptedSyncStatus, 'localVaultAvailable' | 'unlocked'>,
    reason: 'save-password' | 'connect' | 'reveal-password' | 'sync'
  ) {
    return new Promise<boolean>((resolve) => {
      const needsSetup = status.localVaultAvailable === false
      let password = ''
      let confirmation = ''
      let errorMessage = ''
      let submitting = false
      let settled = false
      let passwordDialog: ReturnType<typeof options.modal.confirm>
      const copy = secureDataPromptCopy(needsSetup, reason)

      const finish = (value: boolean) => {
        if (settled) return
        settled = true
        resolve(value)
      }

      const submit = async () => {
        if (submitting) return Promise.reject(new Error(t('hosts:unlockingSecureData')))
        if (needsSetup && password.length < 8) {
          errorMessage = t('hosts:useAtLeast8CharactersForYourEncryptionPassword')
          updateDialog()
          return Promise.reject(new Error(errorMessage))
        }
        if (!needsSetup && !password.trim()) {
          errorMessage = t('hosts:enterYourEncryptionPassword')
          updateDialog()
          return Promise.reject(new Error(errorMessage))
        }
        if (needsSetup && password !== confirmation) {
          errorMessage = t('hosts:theEncryptionPasswordsDoNotMatch')
          updateDialog()
          return Promise.reject(new Error(errorMessage))
        }
        submitting = true
        errorMessage = ''
        updateDialog()
        options.setBusyAction('sync')
        try {
          if (needsSetup) {
            await SetUpSecureData(password)
          } else {
            await UnlockSecureData(password)
          }
          await loadSyncStatus()
          passwordDialog.destroy()
          finish(true)
        } catch (error) {
          errorMessage = readableEncryptedSyncError(error)
          updateDialog()
          throw error
        } finally {
          submitting = false
          options.setBusyAction(null)
          if (!settled) updateDialog()
        }
      }

      const renderContent = () => (
        <div className="sync-password-dialog">
          <p id={`${id}-description`} className="secure-data-description">
            {copy.description}
          </p>
          <div className="sync-password-dialog-fields">
            <div className="secure-data-field">
              <label htmlFor={`${id}-password`}>{t('hosts:encryptionPassword')}</label>
              <PasswordInput
                id={`${id}-password`}
                autoFocus
                aria-invalid={!!errorMessage}
                aria-describedby={errorMessage ? `${id}-error` : undefined}
                autoComplete={needsSetup ? 'new-password' : 'current-password'}
                defaultValue={password}
                disabled={submitting}
                onChange={(event) => {
                  password = event.target.value
                  if (errorMessage) {
                    errorMessage = ''
                    updateDialog()
                  }
                }}
              />
            </div>
            {needsSetup && (
              <div className="secure-data-field">
                <label htmlFor={`${id}-confirmation`}>{t('hosts:confirmEncryptionPassword')}</label>
                <PasswordInput
                  id={`${id}-confirmation`}
                  aria-invalid={!!errorMessage}
                  aria-describedby={errorMessage ? `${id}-error` : `${id}-note`}
                  autoComplete="new-password"
                  defaultValue={confirmation}
                  disabled={submitting}
                  onChange={(event) => {
                    confirmation = event.target.value
                    if (errorMessage) {
                      errorMessage = ''
                      updateDialog()
                    }
                  }}
                />
              </div>
            )}
          </div>
          {needsSetup && (
            <p id={`${id}-note`} className="sync-dialog-note">
              {t('hosts:encryptionPasswordRecoveryWarning')}
            </p>
          )}
          {errorMessage && (
            <div id={`${id}-error`} className="sync-dialog-error" role="alert">
              {errorMessage}
            </div>
          )}
        </div>
      )

      const updateDialog = () => {
        if (settled) return
        passwordDialog.update({
          content: renderContent(),
          okButtonProps: { loading: submitting }
        })
      }

      passwordDialog = options.modal.confirm({
        className: 'secure-data-alert',
        overlayClassName: 'secure-data-overlay',
        icon: <img className="secure-data-icon" src="./appicon.png" alt="" draggable={false} />,
        descriptionId: `${id}-description`,
        submitOnEnter: true,
        title: copy.title,
        content: renderContent(),
        okText: copy.okText,
        okButtonProps: { loading: submitting },
        onOk: submit,
        onCancel: () => finish(false)
      })
    })
  }

  return { ensureSecureDataUnlocked }
}
function secureDataPromptCopy(
  needsSetup: boolean,
  reason: 'save-password' | 'connect' | 'reveal-password' | 'sync'
) {
  if (needsSetup) {
    return {
      title: t('hosts:createEncryptionPassword'),
      okText: t('hosts:createPassword'),
      description: t('hosts:createEncryptionPasswordDescription')
    }
  }
  if (reason === 'connect') {
    return {
      title: t('hosts:unlockToConnect'),
      okText: t('hosts:unlockAndConnect'),
      description: t('hosts:enterYourEncryptionPasswordToUseTheSavedSshPasswordForThisHost')
    }
  }
  if (reason === 'reveal-password') {
    return {
      title: t('hosts:unlockSavedPasswords'),
      okText: t('hosts:unlock'),
      description: t('hosts:enterYourEncryptionPasswordToViewOrCopySavedSshPasswordsOnThisMac')
    }
  }
  if (reason === 'save-password') {
    return {
      title: t('hosts:unlockSecureData'),
      okText: t('hosts:unlock'),
      description: t('hosts:enterYourEncryptionPasswordToSaveThisSshPasswordInProtectedData')
    }
  }
  return {
    title: t('hosts:unlockSecureData'),
    okText: t('hosts:unlock'),
    description: t('hosts:enterYourEncryptionPasswordToContinue')
  }
}

const readableEncryptedSyncError = (error: unknown) =>
  error instanceof Error ? error.message : t('hosts:unableToUnlockSecureData')
