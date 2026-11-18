import assert from 'node:assert/strict'
import test from 'node:test'
import {
  tunnelAddress,
  tunnelNeedsRestart,
  validateTunnel,
  localAddressIsLoopback
} from '../../plugins/ssh-tunnels/src/ui/tunnelModel'
import type { TunnelStatus } from '../../plugins/ssh-tunnels/src/ui/types'
const tunnel: TunnelStatus = {
  id: 'fixture',
  name: 'Database',
  hostID: 'host',
  localAddress: '127.0.0.1',
  localPort: 13306,
  targetAddress: 'db.internal',
  targetPort: 3306,
  createdAt: '',
  updatedAt: '',
  state: 'running',
  error: '',
  lastError: '',
  activeConfig: null
}

test('formats IPv6 endpoints without losing the port boundary', () => {
  assert.equal(tunnelAddress('::1', 3306), '[::1]:3306')
  assert.equal(tunnelAddress('db.internal', 3306), 'db.internal:3306')
})
test('restart notice compares forwarding settings against the actual running configuration', () => {
  assert.equal(tunnelNeedsRestart(tunnel), false)
  assert.equal(tunnelNeedsRestart({ ...tunnel, name: 'Renamed', activeConfig: tunnel }), false)
  assert.equal(tunnelNeedsRestart({ ...tunnel, localPort: 23306, activeConfig: tunnel }), true)
  assert.equal(tunnelNeedsRestart({ ...tunnel, hostID: 'other', activeConfig: tunnel }), true)
})
test('rejects invalid ports and URL targets before saving', () => {
  assert.equal(validateTunnel(tunnel), null)
  for (const port of [0, -1, 65536, 1.5, NaN])
    assert.ok(validateTunnel({ ...tunnel, localPort: port }))
  assert.ok(validateTunnel({ ...tunnel, targetAddress: 'https://db.internal' }))
  assert.ok(validateTunnel({ ...tunnel, hostID: '' }))
})
test('only identifies local loopback addresses as private listeners', () => {
  assert.equal(localAddressIsLoopback('127.0.0.1'), true)
  assert.equal(localAddressIsLoopback('127.1.2.3'), true)
  assert.equal(localAddressIsLoopback('::1'), true)
  assert.equal(localAddressIsLoopback('0.0.0.0'), false)
  assert.equal(localAddressIsLoopback('192.168.1.5'), false)
})
