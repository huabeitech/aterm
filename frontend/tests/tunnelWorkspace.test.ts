import assert from 'node:assert/strict'
import test from 'node:test'
import { createWorkspaceTab, addWorkspaceTab, nextWorkspaceTabAfterClose } from '../src/renderer/src/features/workspace/workspaceModel'

test('global tunnels use one closable workspace and return to the previous connection', () => {
 const connection = createWorkspaceTab('terminal:server')
 const tunnels = createWorkspaceTab('tunnels')
 const tabs = addWorkspaceTab(addWorkspaceTab([connection], tunnels), tunnels)
 assert.equal(tabs.length, 2)
 assert.equal(tunnels.kind, 'tunnels')
 assert.equal(tunnels.closable, true)
 assert.equal(nextWorkspaceTabAfterClose(tabs, 'tunnels', ['terminal:server'], ['tunnels']), 'terminal:server')
})
