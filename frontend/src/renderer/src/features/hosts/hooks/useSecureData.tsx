import {
  encryptionPasswordCopy,
  encryptionPasswordError,
  type EncryptionPasswordValidation
} from '../../../../../shared/security/passwordPrompts'
import { i18n, t } from '../../../../../shared/i18n'
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
  async function ensureSecureDataUnlocked() {
    const status = await loadSyncStatus()
    if (status.unlocked) return true
    return promptSecureDataUnlock(status)
  }
  function promptSecureDataUnlock(
    status: Pick<domain.EncryptedSyncStatus, 'localVaultAvailable' | 'unlocked'>
  ) {
    return new Promise<boolean>((resolve) => {
      const needsSetup = status.localVaultAvailable === false
      let password = ''
      let confirmation = ''
      let error: unknown = null
      let validationKey: EncryptionPasswordValidation | null = null
      let submitting = false
      let settled = false
      let passwordDialog: ReturnType<typeof options.modal.confirm>

      const finish = (value: boolean) => {
        if (settled) return
        settled = true
        i18n.off('languageChanged', updateDialog)
        resolve(value)
      }

      const submit = async () => {
        if (submitting) return Promise.reject(new Error(t('security:unlockTitle')))
        if (needsSetup && password.length < 8) {
          validationKey = 'encryptionPasswordTooShort'
          updateDialog()
          return Promise.reject(new Error(renderError()))
        }
        if (!needsSetup && !password.trim()) {
          validationKey = 'enterEncryptionPassword'
          updateDialog()
          return Promise.reject(new Error(renderError()))
        }
        if (needsSetup && password !== confirmation) {
          validationKey = 'encryptionPasswordsDoNotMatch'
          updateDialog()
          return Promise.reject(new Error(renderError()))
        }
        submitting = true
        error = null
        validationKey = null
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
        } catch (cause) {
          error = cause
          updateDialog()
          throw cause
        } finally {
          submitting = false
          options.setBusyAction(null)
          if (!settled) updateDialog()
        }
      }

      const renderError = () =>
        validationKey
          ? t(`security:${validationKey}`)
          : error ? encryptionPasswordError(error) : ''

      const renderContent = () => (
        <div className="sync-password-dialog">
          <p id={`${id}-description`} className="secure-data-description">
            {encryptionPasswordCopy(needsSetup ? 'setup' : 'unlock').description}
          </p>
          <div className="sync-password-dialog-fields">
            <div className="secure-data-field">
              <label htmlFor={`${id}-password`}>{t('security:encryptionPassword')}</label>
              <PasswordInput
                id={`${id}-password`}
                autoFocus
                aria-invalid={!!(validationKey || error)}
                aria-describedby={validationKey || error ? `${id}-error` : undefined}
                autoComplete={needsSetup ? 'new-password' : 'current-password'}
                defaultValue={password}
                disabled={submitting}
                onChange={(event) => {
                  password = event.target.value
                  if (validationKey || error) {
                    error = null
                    validationKey = null
                    updateDialog()
                  }
                }}
              />
            </div>
            {needsSetup && (
              <div className="secure-data-field">
                <label htmlFor={`${id}-confirmation`}>{t('security:confirmEncryptionPassword')}</label>
                <PasswordInput
                  id={`${id}-confirmation`}
                  aria-invalid={!!(validationKey || error)}
                  aria-describedby={validationKey || error ? `${id}-error` : undefined}
                  autoComplete="new-password"
                  defaultValue={confirmation}
                  disabled={submitting}
                  onChange={(event) => {
                    confirmation = event.target.value
                    if (validationKey || error) {
                      error = null
                      validationKey = null
                      updateDialog()
                    }
                  }}
                />
              </div>
            )}
          </div>
          {!!(validationKey || error) && (
            <div id={`${id}-error`} className="sync-dialog-error" role="alert">
              {renderError()}
            </div>
          )}
        </div>
      )

      const updateDialog = () => {
        if (settled) return
        const copy = encryptionPasswordCopy(needsSetup ? 'setup' : 'unlock')
        passwordDialog.update({
          title: copy.title,
          okText: copy.okText,
          content: renderContent(),
          okButtonProps: { loading: submitting }
        })
      }

      const copy = encryptionPasswordCopy(needsSetup ? 'setup' : 'unlock')
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
      i18n.on('languageChanged', updateDialog)
    })
  }

  return { ensureSecureDataUnlocked }
}
