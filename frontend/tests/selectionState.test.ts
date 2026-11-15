import test from 'node:test'
import assert from 'node:assert/strict'
import * as selection from '../src/renderer/src/components/hosts/selectionState'
test('treats a selected host outside the current list as unselected', () => {
    const {getVisibleSelectedHost} = selection;
    const visibleHosts = [{id: 'host-b', alias: 'Host B'}];

    const selected = getVisibleSelectedHost(visibleHosts, 'host-a');

    assert.equal(selected, null);
});

test('single click selects only the clicked host and anchors selection', () => {
    const {updateHostSelection} = selection;
    const visibleHosts = [{id: 'host-a'}, {id: 'host-b'}, {id: 'host-c'}];

    const next = updateHostSelection({
        visibleHosts,
        selectedIDs: ['host-a', 'host-c'],
        anchorID: 'host-a',
        clickedID: 'host-b',
        additive: false,
        range: false,
    });

    assert.deepEqual(next, {selectedIDs: ['host-b'], anchorID: 'host-b'});
});

test('command click toggles one host without clearing the existing selection', () => {
    const {updateHostSelection} = selection;
    const visibleHosts = [{id: 'host-a'}, {id: 'host-b'}, {id: 'host-c'}];

    const added = updateHostSelection({
        visibleHosts,
        selectedIDs: ['host-a'],
        anchorID: 'host-a',
        clickedID: 'host-c',
        additive: true,
        range: false,
    });
    const removed = updateHostSelection({
        visibleHosts,
        selectedIDs: added.selectedIDs,
        anchorID: added.anchorID,
        clickedID: 'host-a',
        additive: true,
        range: false,
    });

    assert.deepEqual(added, {selectedIDs: ['host-a', 'host-c'], anchorID: 'host-c'});
    assert.deepEqual(removed, {selectedIDs: ['host-c'], anchorID: 'host-a'});
});

test('shift click selects the visible range from the anchor to the clicked host', () => {
    const {updateHostSelection} = selection;
    const visibleHosts = [{id: 'host-a'}, {id: 'host-b'}, {id: 'host-c'}, {id: 'host-d'}];

    const next = updateHostSelection({
        visibleHosts,
        selectedIDs: ['host-a'],
        anchorID: 'host-a',
        clickedID: 'host-c',
        additive: false,
        range: true,
    });

    assert.deepEqual(next, {selectedIDs: ['host-a', 'host-b', 'host-c'], anchorID: 'host-a'});
});

test('dragging an unselected host moves only that host, while dragging a selected host moves the selection', () => {
    const {getHostDragIDs} = selection;

    assert.deepEqual(getHostDragIDs(['host-a', 'host-c'], 'host-b'), ['host-b']);
    assert.deepEqual(getHostDragIDs(['host-a', 'host-c'], 'host-c'), ['host-a', 'host-c']);
});
