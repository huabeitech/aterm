import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  applyViewAction,
  isViewToggleSelected
} from '../src/renderer/src/features/plugins/viewActions'
import { pluginPresentation } from '../src/renderer/src/features/plugins/presentation'
const view = {
  id: 'inventory.panel',
  title: 'Inventory',
  entry: 'ui/inventory.html',
  placement: 'terminal.sidePanel'
}
test('toggle selection follows the actual open panel, including section navigation and close', () => {
  const toggle = { type: 'view.toggle', view: view.id, context: { section: 'overview' } } as const
  const command = { id: 'inventory.show', title: 'Inventory', action: toggle }
  assert.equal(isViewToggleSelected(command), false)
  const opened = applyViewAction(undefined, toggle, 'first')!
  assert.equal(isViewToggleSelected(command, opened), true)
  const section = applyViewAction(
    opened,
    { type: 'view.open', view: view.id, context: { section: 'stock' } },
    'second'
  )!
  assert.equal(isViewToggleSelected(command, section), true)
  assert.deepEqual(section.context, { section: 'stock' })
  assert.equal(section.invocationId, 'second')
  assert.equal(applyViewAction(section, toggle, 'third'), undefined)
  assert.equal(applyViewAction(section, { type: 'view.close', view: view.id }, 'fourth'), undefined)
  assert.equal(
    applyViewAction(section, { type: 'view.close', view: 'other.panel' }, 'fifth'),
    section
  )
  assert.equal(
    isViewToggleSelected({ ...command, action: { type: 'view.open', view: view.id } }, section),
    undefined
  )
  const replacement = applyViewAction(
    section,
    { type: 'view.toggle', view: 'other.panel' },
    'sixth'
  )!
  assert.equal(replacement.id, 'other.panel')
  assert.equal(isViewToggleSelected(command, replacement), false)
})

test('connecting and first refresh are loading; errors appear only after a failed state', () => {
  assert.equal(pluginPresentation('connecting', 'inactive'), 'loading')
  assert.equal(pluginPresentation('connecting', 'active', 'delayed'), 'loading')
  assert.equal(pluginPresentation('running', 'inactive'), 'loading')
  assert.equal(pluginPresentation('running', 'active', 'loading'), 'loading')
  assert.equal(pluginPresentation('running', 'active', 'ready'), 'ready')
  assert.equal(pluginPresentation('running', 'active', 'ready', true), 'paused')
  assert.equal(pluginPresentation('running', 'active', 'delayed'), 'delayed')
  assert.equal(pluginPresentation('running', 'active', 'unavailable'), 'delayed')
  assert.equal(pluginPresentation('running', 'active', 'unsupported'), 'unsupported')
  assert.equal(
    pluginPresentation('running', 'active', 'permission-required'),
    'permission-required'
  )
  assert.equal(pluginPresentation('running', 'failed', 'ready'), 'failed')
  assert.equal(pluginPresentation('error', 'active', 'ready'), 'disconnected')
  assert.equal(pluginPresentation('exited', 'active', 'ready'), 'disconnected')
})
