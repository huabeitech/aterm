import assert from 'node:assert/strict'
import test from 'node:test'
import { createWorkspaceTab, addWorkspaceTab, nextWorkspaceTabAfterClose } from '../src/renderer/src/features/workspace/workspaceModel'

test('global tunnels use one closable workspace and return to the previous connection', () => {
 const connection = createWorkspaceTab('terminal:server')
 const tunnels = createWorkspaceTab('plugin:huabei.ssh-tunnels:tunnels.workspace')
 const tabs = addWorkspaceTab(addWorkspaceTab([connection], tunnels), tunnels)
 assert.equal(tabs.length, 2)
 assert.equal(tunnels.kind, 'plugin')
 assert.equal(tunnels.closable, true)
 assert.equal(nextWorkspaceTabAfterClose(tabs, 'plugin:huabei.ssh-tunnels:tunnels.workspace', ['terminal:server'], ['plugin:huabei.ssh-tunnels:tunnels.workspace']), 'terminal:server')
})
