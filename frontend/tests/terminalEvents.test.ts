import { test } from 'node:test'
import assert from 'node:assert/strict'
import { terminalEvents } from '../src/renderer/src/api/terminalEvents'
import type { TerminalEvent } from '../src/shared/contracts/terminal'

test('One IPC listener routes data to its session and broadcasts accessibility changes', () => {
  let dispatch!: (event: TerminalEvent) => void
  let starts = 0,
    stops = 0
  const subscribe = terminalEvents((callback) => {
    dispatch = callback
    starts++
    return () => {
      stops++
    }
  })
  const global: TerminalEvent[] = [],
    a: TerminalEvent[] = [],
    b: TerminalEvent[] = []
  const off = [
    subscribe((event) => global.push(event)),
    subscribe((event) => a.push(event), 'a'),
    subscribe((event) => b.push(event), 'b')
  ]
  const packet: TerminalEvent = { type: 'data', id: 'a', seq: 1, data: 'test' }
  dispatch(packet)
  assert.equal(starts, 1)
  assert.deepEqual(a, [packet])
  assert.deepEqual(b, [])
  assert.deepEqual(global, [packet])
  dispatch({ type: 'accessibility', enabled: true })
  assert.equal(a.length, 2)
  assert.equal(b.length, 1)
  off[1]()
  off[1]()
  dispatch(packet)
  assert.equal(a.length, 2)
  off[0]()
  off[2]()
  assert.equal(stops, 1)
  const unsub = subscribe(() => {}, 'a')
  assert.equal(starts, 2)
  unsub()
  assert.equal(stops, 2)
})

test('Removing a subscriber during dispatch and a failing callback cannot block another pane', (t) => {
  t.mock.method(console, 'error', () => {})
  let dispatch!: (event: TerminalEvent) => void
  const subscribe = terminalEvents((callback) => {
    dispatch = callback
    return () => {}
  })
  let lateCalls = 0,
    healthyCalls = 0
  let remove = () => {}
  const off = subscribe(() => {
    remove()
    throw Error('Subscriber closed')
  }, 'a')
  remove = subscribe(() => {
    lateCalls++
  }, 'a')
  const healthy = subscribe(() => {
    healthyCalls++
  }, 'a')
  dispatch({ type: 'data', id: 'a', seq: 1, data: 'test' })
  assert.equal(lateCalls, 0)
  assert.equal(healthyCalls, 1)
  off()
  healthy()
})

test('Routing load stays at one pane callback per packet with 1, 8 and 32 sessions', (t) => {
  for (const count of [1, 8, 32]) {
    let dispatch!: (event: TerminalEvent) => void
    let calls = 0
    const subscribe = terminalEvents((callback) => {
      dispatch = callback
      return () => {}
    })
    const off = Array.from({ length: count }, (_, index) =>
      subscribe(() => {
        calls++
      }, String(index))
    )
    const started = performance.now()
    for (let i = 0; i < 30000; i++)
      dispatch({ type: 'data', id: String(i % count), seq: i + 1, data: 'x'.repeat(64) })
    assert.equal(calls, 30000)
    t.diagnostic(
      JSON.stringify({
        sessions: count,
        packets: 30000,
        callbacks: calls,
        previousFanoutCallbacks: 30000 * count,
        dispatchMs: Number((performance.now() - started).toFixed(2))
      })
    )
    off.forEach((unsubscribe) => unsubscribe())
  }
})
