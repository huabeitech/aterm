import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  addWorkspaceTab,
 removeWorkspaceTabs,
  createWorkspaceTab,
  openHostWorkspaceTab,
  nextWorkspaceTabAfterClose,
  reorderWorkspaceTabs,
  adjacentWorkspaceTab
} from '../src/renderer/src/features/workspace/workspaceModel'
test('Workspace tabs contain working sessions and settings, with home as the empty fallback', () => {
  const settings = createWorkspaceTab('settings'),
    terminal = createWorkspaceTab('terminal:a')
  const tabs = addWorkspaceTab([settings], settings)
  assert.equal(tabs.length, 1)
  const multiple = [...tabs, terminal]
  assert.deepEqual(
    reorderWorkspaceTabs(multiple, 'terminal:a', 'settings', false).map((t) => t.id),
    ['terminal:a', 'settings']
  )
  assert.equal(adjacentWorkspaceTab(multiple, 'settings', -1), 'terminal:a')
  assert.equal(
    nextWorkspaceTabAfterClose(multiple, 'settings', ['terminal:a'], ['settings']),
    'terminal:a'
  )
  assert.equal(nextWorkspaceTabAfterClose([terminal], 'terminal:a', [], ['terminal:a']), 'home')
})

test('explicit host sessions append independent workspaces', () => {
  const settings = createWorkspaceTab('settings')
  const first = openHostWorkspaceTab([settings], 'alpha')
  const second = openHostWorkspaceTab(first.tabs, 'alpha')
  assert.deepEqual(second.tabs, [settings, first.tab, second.tab])
  assert.equal(second.tab.hostID, 'alpha')
  assert.notEqual(first.tab.id, second.tab.id)
})

test('tab removal preserves the source list and close follows recent history or its left neighbor', () => {
  const tabs=[createWorkspaceTab('settings'),createWorkspaceTab('terminal:a'),createWorkspaceTab('terminal:b')]
  assert.deepEqual(removeWorkspaceTabs(tabs,['terminal:b']).map(tab=>tab.id),['settings','terminal:a'])
  assert.equal(tabs.length,3)
  assert.equal(nextWorkspaceTabAfterClose(tabs,'terminal:b',['settings','terminal:a'],['terminal:b']),'settings')
  assert.equal(nextWorkspaceTabAfterClose(tabs,'terminal:b',[],['terminal:b']),'terminal:a')
})
test('movable workspace tabs reorder before/after targets and cycle at both ends', () => {
  const tabs=[createWorkspaceTab('settings'),createWorkspaceTab('terminal:a'),createWorkspaceTab('terminal:b')]
  assert.deepEqual(reorderWorkspaceTabs(tabs,'terminal:b','terminal:a',false).map(tab=>tab.id),['settings','terminal:b','terminal:a'])
  assert.deepEqual(reorderWorkspaceTabs(tabs,'settings','terminal:b',true).map(tab=>tab.id),['terminal:a','terminal:b','settings'])
  assert.equal(adjacentWorkspaceTab(tabs,'settings',-1),'terminal:b')
  assert.equal(adjacentWorkspaceTab(tabs,'terminal:b',1),'settings')
})
