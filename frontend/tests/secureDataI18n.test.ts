import assert from 'node:assert/strict'
import test from 'node:test'
import { i18n, t } from '../src/shared/i18n'
import { resources } from '../src/shared/i18n/resources'
import {
  encryptionPasswordCopy,
  encryptionPasswordError,
  sshPrivateKeyPromptCopy
} from '../src/shared/security/passwordPrompts'

const privateKeyPrompt = { kind: 'credentials', title: 'Unlock Private Key' }

test('every locale supplies matching centralized password copy and parameters', () => {
  const entries = Object.entries(resources.en.security)
  for (const [locale, resource] of Object.entries(resources)) {
    assert.deepEqual(Object.keys(resource.security).sort(), Object.keys(resources.en.security).sort(), locale)
    for (const [key, source] of entries) {
      const translated = resource.security[key as keyof typeof resource.security]
      assert.ok(translated.trim(), `${locale}:${key}`)
      assert.deepEqual(translated.match(/{{\w+}}/g), source.match(/{{\w+}}/g), `${locale}:${key}`)
    }
    for (const obsolete of ['unlockConnectionCredentialsDescription', 'unlockManagedSshKeysDescription', 'unlockToConnect']) {
      assert.ok(!Object.hasOwn(resource.hosts, obsolete), `${locale}:${obsolete}`)
    }
    assert.ok(!Object.hasOwn(resource.security, 'backupPassword'), locale)
  }
})

test('all password actions and validation copy follow the current language', async () => {
  const invalidPassword = new Error('invalid encryption password')
  try {
    await i18n.changeLanguage('en')
    assert.deepEqual(encryptionPasswordCopy('unlock'), {
      title: 'Unlock aTerm', description: 'Enter your encryption password to continue.', okText: 'Unlock'
    })
    assert.equal(encryptionPasswordCopy('restore').title, 'Restore Encrypted Backup')
    assert.match(encryptionPasswordCopy('restore').description, /when this backup was created/)
    assert.equal(sshPrivateKeyPromptCopy(privateKeyPrompt)?.fieldLabel, 'SSH private key passphrase')
    assert.equal(encryptionPasswordError(invalidPassword), 'Incorrect encryption password. Try again.')
    await i18n.changeLanguage('zh-CN')
    assert.deepEqual(encryptionPasswordCopy('unlock'), {
      title: '解锁 aTerm', description: '输入加密密码以继续。', okText: '解锁'
    })
    assert.deepEqual(encryptionPasswordCopy('restore'), {
      title: '恢复加密备份', description: '输入创建此备份时使用的加密密码。', okText: '恢复'
    })
    assert.equal(encryptionPasswordCopy('setup').okText, '设置')
    assert.match(encryptionPasswordCopy('setup').description, /忘记后无法找回/)
    assert.equal(encryptionPasswordCopy('change').okText, '更改')
    assert.equal(t('security:currentEncryptionPassword'), '当前加密密码')
    assert.equal(t('security:newEncryptionPasswordsDoNotMatch'), '两次输入的新加密密码不一致。')
    assert.equal(encryptionPasswordError(invalidPassword), '加密密码不正确，请重试。')
    assert.equal(encryptionPasswordError(new Error('vault is locked')), 'aTerm 已锁定，解锁后可继续。')
    assert.deepEqual(sshPrivateKeyPromptCopy(privateKeyPrompt), {
      title: '解锁 SSH 私钥', description: '输入此 SSH 私钥的口令。', fieldLabel: 'SSH 私钥口令', okText: '解锁'
    })
  } finally {
    await i18n.changeLanguage('en')
  }
})

test('password presentation preserves unrelated diagnostics and server prompts', () => {
  assert.equal(sshPrivateKeyPromptCopy({ kind: 'credentials', title: 'Company MFA' }), null)
  assert.equal(sshPrivateKeyPromptCopy({ kind: 'host-key', title: 'Unlock Private Key' }), null)
  const diagnostic = 'protected data failed integrity verification'
  assert.equal(encryptionPasswordError(new Error(diagnostic)), diagnostic)
})
