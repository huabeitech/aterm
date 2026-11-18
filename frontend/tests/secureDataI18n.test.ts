import assert from 'node:assert/strict'
import test from 'node:test'
import { i18n, t } from '../src/shared/i18n'
import { resources } from '../src/shared/i18n/resources'

const keys = [
  'unlockManagedSshKeys',
  'unlockManagedSshKeysDescription',
  'unlockConnectionCredentialsDescription'
] as const

test('SSH credential unlock copy is translated in every locale', () => {
  for (const [locale, resource] of Object.entries(resources)) {
    for (const key of keys) {
      assert.ok(resource.hosts[key].trim(), `${locale}:${key}`)
      assert.deepEqual(
        resource.hosts[key].match(/{{\w+}}/g),
        resources.en.hosts[key].match(/{{\w+}}/g),
        `${locale}:${key}`
      )
    }
  }
})

test('SSH credential unlock copy follows language changes', async () => {
  try {
    await i18n.changeLanguage('en')
    assert.equal(t('hosts:unlockManagedSshKeys'), 'Unlock SSH Keys')
    assert.match(t('hosts:unlockConnectionCredentialsDescription'), /keys/)
    await i18n.changeLanguage('zh-CN')
    assert.equal(t('hosts:unlockManagedSshKeys'), '解锁 SSH 密钥')
    assert.match(t('hosts:unlockConnectionCredentialsDescription'), /密钥/)
  } finally {
    await i18n.changeLanguage('en')
  }
})
