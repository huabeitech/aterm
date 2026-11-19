import assert from 'node:assert/strict'
import test from 'node:test'
import { themeOptions } from '../src/renderer/src/app/themes'
import { themeDefinition } from '../src/renderer/src/app/themeRegistry'
import { resolveTheme } from '../src/shared/contracts/resolvedTheme'
import { isThemeID } from '../src/shared/contracts/themes'
import { i18n } from '../src/shared/i18n'
import { resources } from '../src/shared/i18n/resources'

const mixedID = 'light-dark-terminal'

test('the mixed theme resolves to light UI colors and the complete dark terminal palette', () => {
  const definition = themeDefinition(mixedID)
  assert.ok(definition)
  assert.ok(isThemeID(mixedID))
  const mixed = resolveTheme(definition, 'dark')
  const light = resolveTheme(themeDefinition('light'), 'light')
  const dark = resolveTheme(themeDefinition('dark'), 'dark')
  assert.equal(mixed.appearance, 'light')
  for (const [token, color] of Object.entries(light.cssVariables)) {
    const expected = token.startsWith('--terminal-') ? dark.cssVariables[token as `--${string}`] : color
    assert.equal(mixed.cssVariables[token as `--${string}`], expected, token)
  }
  assert.deepEqual(mixed.terminal, dark.terminal)
})

test('custom themes resolve app and terminal colors independently', () => {
  const base = { id: 'custom', name: 'Custom', appearance: 'light' } as const
  const light = resolveTheme(themeDefinition('light'), 'light')
  const colors = { 'surface.window': '#eeeeee', 'text.primary': '#222222' }
  const terminal = { background: '#111111', foreground: '#dddddd' }

  const appOnly = resolveTheme({ ...base, colors }, 'light')
  assert.equal(appOnly.cssVariables['--app-window-background'], colors['surface.window'])
  assert.deepEqual(appOnly.terminal, light.terminal)

  const terminalOnly = resolveTheme({ ...base, terminal }, 'light')
  assert.equal(terminalOnly.cssVariables['--app-window-background'], light.cssVariables['--app-window-background'])
  assert.equal(terminalOnly.cssVariables['--mac-text'], light.cssVariables['--mac-text'])
  assert.equal(terminalOnly.cssVariables['--terminal-background'], terminal.background)

  const both = resolveTheme({ ...base, colors, terminal }, 'light')
  assert.deepEqual(both.terminal, terminalOnly.terminal)
  assert.equal(both.cssVariables['--mac-text'], colors['text.primary'])
})

test('the mixed theme keeps its English name across languages', async () => {
  for (const resource of Object.values(resources)) {
    assert.equal(resource.settings.lightDarkTerminalTheme, 'Daylight')
  }
  const previous = i18n.language
  try {
    await i18n.changeLanguage('en')
    assert.equal(themeOptions().find(({ value }) => value === mixedID)?.label, 'Daylight')
    await i18n.changeLanguage('zh-CN')
    assert.equal(themeOptions().find(({ value }) => value === mixedID)?.label, 'Daylight')
  } finally {
    await i18n.changeLanguage(previous)
  }
})
