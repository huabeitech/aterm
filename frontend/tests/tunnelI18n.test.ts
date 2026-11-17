import assert from 'node:assert/strict'
import test from 'node:test'
import { i18n, t } from '../src/shared/i18n'
import { resources } from '../src/shared/i18n/resources'
import { validateTunnel } from '../src/renderer/src/features/tunnels/tunnelModel'
import { tunnelErrorMessage } from '../src/renderer/src/features/tunnels/tunnelErrors'

test('every locale supplies matching tunnel keys and interpolation parameters', () => {
  const entries = Object.entries(resources.en.hosts).filter(([key]) => key.startsWith('tunnels.'))
  for (const [locale, resource] of Object.entries(resources)) {
    assert.deepEqual(
      Object.keys(resource.hosts)
        .filter((key) => key.startsWith('tunnels.'))
        .sort(),
      entries.map(([key]) => key).sort(),
      locale
    )
    for (const [key, source] of entries) {
      const translated = resource.hosts[key as keyof typeof resource.hosts]
      assert.ok(translated.trim(), `${locale}:${key}`)
      assert.deepEqual(
        translated.match(/{{\w+}}/g)?.sort(),
        source.match(/{{\w+}}/g)?.sort(),
        `${locale}:${key}`
      )
    }
  }
})

test('tunnel labels and existing validation results follow the current language', async () => {
  try {
    await i18n.changeLanguage('en')
    assert.equal(t('hosts:tunnels.title'), 'Tunnels')
    const validation = validateTunnel({
      name: '',
      hostID: '',
      localAddress: '',
      localPort: 0,
      targetAddress: '',
      targetPort: 0
    })!
    await i18n.changeLanguage('zh-CN')
    assert.equal(t('hosts:tunnels.title'), '隧道')
    assert.equal(tunnelErrorMessage(validation), t('hosts:tunnels.nameRequired'))
    assert.notEqual(tunnelErrorMessage(validation), validation)
    const diagnostic = 'SSH connection: connection refused'
    assert.equal(
      tunnelErrorMessage(diagnostic),
      t('hosts:tunnels.sshFailed', { detail: diagnostic })
    )
    assert.ok(tunnelErrorMessage(diagnostic).includes(diagnostic))
    assert.notEqual(t('hosts:tunnels.success'), resources.en.hosts['tunnels.success'])
  } finally {
    await i18n.changeLanguage('en')
  }
})
