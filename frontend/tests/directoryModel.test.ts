import test from 'node:test'
import assert from 'node:assert/strict'
import * as model from '../src/renderer/src/features/directories/model/directoryModel'
const directory = (id: string, parentID = '', sortOrder = 0) => ({id, parentID, name: id, sortOrder, createdAt: '', updatedAt: ''});

test('builds a depth-first directory tree and full path labels', () => {
    const directories = [directory('child', 'root'), directory('root'), directory('sibling', '', 1)];
    assert.deepEqual(model.buildDirectoryTree(directories).map(({directory, depth}) => [directory.id, depth]), [['root', 0], ['child', 1], ['sibling', 0]]);
    assert.equal(model.directoryLabel('child', directories), 'root / child');
});

test('prevents dropping a directory into its descendant', () => {
    const directories = [directory('root'), directory('child', 'root')];
    assert.equal(model.isDirectoryDescendant('child', 'root', directories), true);
    assert.equal(model.calculateDirectoryDropTarget({clientX: 100, clientY: 10}, {left: 0, top: 0, width: 200, height: 20}, directories[1], 1, 'root', directories), null);
});

test('resolves root, inside, before, and after moves', () => {
    const directories = [directory('a'), directory('b', '', 1), directory('child', 'a')];
    assert.deepEqual(model.resolveDirectoryMove('child', {id: null, position: 'root'}, directories), {parentID: '', sortOrder: 2});
    assert.deepEqual(model.resolveDirectoryMove('b', {id: 'a', position: 'inside'}, directories), {parentID: 'a', sortOrder: 1});
    assert.deepEqual(model.resolveDirectoryMove('b', {id: 'a', position: 'before'}, directories), {parentID: '', sortOrder: 0});
});
