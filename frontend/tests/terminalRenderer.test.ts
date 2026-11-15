import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { WebglAddon } from '@xterm/addon-webgl'
import { terminalRenderer } from '../src/renderer/src/features/terminal/terminalRenderer'

const settle = () => new Promise<void>((resolve) => setImmediate(resolve))
function fakeAddon() {
  let lose = () => {}
  const state = { disposed: 0, timerDisposed: 0, listenerDisposed: 0 }
  const addon = {
    activate() {},
    dispose() {
      state.disposed++
    },
    clearTextureAtlas() {},
    onContextLoss(callback: () => void) {
      lose = callback
      return {
        dispose() {
          state.listenerDisposed++
        }
      }
    },
    _renderer: {
      _cursorBlinkStateManager: {
        dispose() {
          state.timerDisposed++
        }
      }
    }
  } as unknown as WebglAddon
  return { addon, state, lose: () => lose() }
}

test('WebGL recovery waits for a visible pane and stops after repeated losses', async () => {
  let visible = true
  const created: ReturnType<typeof fakeAddon>[] = []
  const renderer = terminalRenderer(
    { rows: 24, refresh() {}, loadAddon() {} },
    () => visible,
    async () => {
      const next = fakeAddon()
      created.push(next)
      return next.addon
    }
  )
  renderer.refresh()
  await settle()
  assert.equal(created.length, 1)
  visible = false
  created[0].lose()
  await settle()
  assert.deepEqual(created[0].state, { disposed: 1, timerDisposed: 1, listenerDisposed: 1 })
  assert.equal(created.length, 1)
  visible = true
  renderer.refresh()
  await settle()
  created[1].lose()
  await settle()
  created[2].lose()
  await settle()
  renderer.refresh()
  await settle()
  assert.equal(created.length, 3)
  renderer.dispose()
  assert.ok(created.every(({ state }) => state.disposed === 1 && state.timerDisposed === 1))
})

test('Stable GPU operation replenishes the recovery budget across separate failures', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const created: ReturnType<typeof fakeAddon>[] = []
  const renderer = terminalRenderer(
    { rows: 24, refresh() {}, loadAddon() {} },
    () => true,
    async () => {
      const next = fakeAddon()
      created.push(next)
      return next.addon
    }
  )
  renderer.refresh()
  await settle()
  for (let i = 0; i < 5; i++) {
    t.mock.timers.tick(30000)
    created.at(-1)!.lose()
    await settle()
  }
  assert.equal(created.length, 6)
  renderer.setEnabled(false)
  assert.equal(created.at(-1)!.state.disposed, 1)
  renderer.refresh()
  await settle()
  assert.equal(created.length, 6)
  renderer.setEnabled(true)
  await settle()
  assert.equal(created.length, 7)
  renderer.dispose()
})

test('Closing a terminal during addon loading cannot attach a late GPU renderer', async () => {
  const next = fakeAddon()
  let resolve!: (addon: WebglAddon) => void
  let attached = false
  const renderer = terminalRenderer(
    {
      rows: 24,
      refresh() {},
      loadAddon() {
        attached = true
      }
    },
    () => true,
    () =>
      new Promise((done) => {
        resolve = done
      })
  )
  renderer.refresh()
  renderer.dispose()
  resolve(next.addon)
  await settle()
  assert.equal(attached, false)
  assert.equal(next.state.disposed, 1)
})

test('Failed GPU activation releases resources and keeps the terminal usable', async (t) => {
  t.mock.method(console, 'warn', () => {})
  const next = fakeAddon()
  let refreshed = 0
  const renderer = terminalRenderer(
    {
      rows: 24,
      refresh() {
        refreshed++
      },
      loadAddon() {
        throw Error('GPU unavailable')
      }
    },
    () => true,
    async () => next.addon
  )
  renderer.refresh()
  await settle()
  assert.equal(next.state.disposed, 1)
  assert.equal(next.state.timerDisposed, 1)
  assert.equal(refreshed, 1)
  renderer.dispose()
  assert.equal(next.state.disposed, 1)
})
