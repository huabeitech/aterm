import assert from 'node:assert/strict'
import test from 'node:test'
import { buildSSHCommand } from '../src/renderer/src/features/hosts/model/sshCommand'
import type { domain } from '../src/shared/contracts/domain'

const host = (identityFile: string): domain.Host => ({
  id: 'production',
  directoryID: '',
  name: 'production',
  alias: 'production',
  hostName: 'server.example',
  user: 'deploy',
  port: 22,
  identityFile,
  passwordAuth: false,
  proxyJump: '',
  jumpHostID: '',
  forwardAgent: false,
  compression: false,
  serverAliveInterval: 0,
  serverAliveCountMax: 0,
  connectTimeout: 0,
  remark: '',
  lastUsedAt: '',
  sortOrders: {},
  createdAt: '',
  updatedAt: ''
})

test('managed keys are omitted from copied OpenSSH commands', () => {
  const value = host('aterm-key://key-id')
  assert.equal(buildSSHCommand(value, [value]), "ssh 'deploy@server.example'")
})

test('file identities remain part of copied OpenSSH commands', () => {
  const value = host('~/.ssh/deploy')
  assert.equal(buildSSHCommand(value, [value]), "ssh -i '~/.ssh/deploy' 'deploy@server.example'")
})
