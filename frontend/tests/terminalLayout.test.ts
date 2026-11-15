import {test} from 'node:test'
import assert from 'node:assert/strict'
import {
  leaf,
  splitPane,
  removePane,
  paneIDs,
  layoutPanes,
  resizeSplit
} from '../src/renderer/src/features/terminal/layout'
test('Terminal layout keeps pane IDs stable across nested splits, resize and close', () => {
  const initial = leaf('a'),
    tree = splitPane(splitPane(initial, 'a', 'b', 'vertical'), 'b', 'c', 'horizontal')
  assert.deepEqual(paneIDs(tree), ['a', 'b', 'c'])
  assert.deepEqual(initial, leaf('a'))
  const layout = layoutPanes(tree)
  assert.equal(layout.panes.length, 3)
  assert.equal(layout.handles.length, 2)
  assert.equal(
    layout.panes.reduce((total, p) => total + p.box.width * p.box.height, 0),
    10000
  )
  const resized = resizeSplit(tree, 'b', 2)
  assert.equal(resized.kind === 'split' && resized.ratio, 0.85)
  const closed = removePane(tree, 'b')!
  assert.deepEqual(paneIDs(closed), ['a', 'c'])
  assert.deepEqual(removePane(removePane(closed, 'a')!, 'c'), null)
})
