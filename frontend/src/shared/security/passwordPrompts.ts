import { t } from '../i18n'

export type EncryptionPasswordValidation = 'enterEncryptionPassword' | 'encryptionPasswordTooShort'
  | 'encryptionPasswordsDoNotMatch' | 'newEncryptionPasswordsDoNotMatch'

export type EncryptionPasswordAction = 'unlock' | 'setup' | 'change' | 'restore'

// Resolve when rendering so open dialogs follow the current language.
export function encryptionPasswordCopy(action: EncryptionPasswordAction) {
  return {
    title: t(`security:${action}Title`),
    description: t(`security:${action}Description`),
    okText: t(`security:${action}Action`)
  }
}

export function encryptionPasswordError(error: unknown): string {
  const diagnostic = error instanceof Error ? error.message : ''
  const detail = error && typeof error === 'object' && 'detail' in error ? error.detail : diagnostic
  if (detail === 'invalid encryption password') return t('security:incorrectEncryptionPassword')
  if (detail === 'vault is locked') return t('security:lockedDescription')
  return diagnostic || t('security:operationFailed')
}

export function sshPrivateKeyPromptCopy(prompt: { kind: string; title: string }) {
  if (prompt.kind !== 'credentials' || prompt.title !== 'Unlock Private Key') return null
  return {
    title: t('security:privateKeyTitle'),
    description: t('security:privateKeyDescription'),
    fieldLabel: t('security:privateKeyPassphrase'),
    okText: t('security:unlockAction')
  }
}
