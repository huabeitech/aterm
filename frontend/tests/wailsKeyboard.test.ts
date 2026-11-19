import test from 'node:test'
import assert from 'node:assert/strict'
import {workspaceKeyCommand} from '../src/renderer/src/api/wailsKeyboard'
import {platformDefaults} from '../src/shared/contracts/preferences'
const preferences=platformDefaults('darwin')
const key = (name: string, extra: Partial<KeyboardEvent> = {}) => ({key:name,metaKey:true,ctrlKey:false,shiftKey:false,altKey:false,isComposing:false,...extra});

test('terminal and normal modes route numbered tabs and configurable commands', () => {
 for (const mode of ['normal','terminal'] as const) {
  assert.equal(workspaceKeyCommand(key('B',{shiftKey:true}),mode,preferences,'darwin'),'toggle-inspector');
  assert.equal(workspaceKeyCommand(key('9'),mode,preferences,'darwin'),'tab-9');
  assert.equal(workspaceKeyCommand(key('t'),mode,preferences,'darwin'),'new-terminal');
  assert.equal(workspaceKeyCommand(key('E',{shiftKey:true}),mode,preferences,'darwin'),'next-connection-view');
  assert.equal(workspaceKeyCommand(key('t',{altKey:true}),mode,preferences,'darwin'),undefined);
 }
});
test('editable fields retain native edits and only permit global workspace commands', () => {
 assert.equal(workspaceKeyCommand(key('p'),'editable',preferences,'darwin'),'search');
 assert.equal(workspaceKeyCommand(key('E',{shiftKey:true}),'editable',preferences,'darwin'),'next-connection-view');
 assert.equal(workspaceKeyCommand(key('t'),'editable',preferences,'darwin'),undefined);
 assert.equal(workspaceKeyCommand(key('c'),'editable',{...preferences,commandPaletteShortcut:'meta+c'},'darwin'),undefined);
 assert.equal(workspaceKeyCommand(key('z',{shiftKey:true}),'editable',{...preferences,commandPaletteShortcut:'meta+shift+z'},'darwin'),undefined);
 assert.equal(workspaceKeyCommand(key('y',{metaKey:false,ctrlKey:true}),'editable',{...preferences,commandPaletteShortcut:'ctrl+y'},'win32'),undefined);
});
test('recording, composition and editor ownership constrain command routing', () => {
 assert.equal(workspaceKeyCommand(key('t'),'recorder',preferences,'darwin'),undefined);
 assert.equal(workspaceKeyCommand(key('t',{isComposing:true}),'terminal',preferences,'darwin'),undefined);
 assert.equal(workspaceKeyCommand(key('Process'),'terminal',preferences,'darwin'),undefined);
 assert.equal(workspaceKeyCommand(key('t'),'editable',preferences,'darwin',true),undefined);
 assert.equal(workspaceKeyCommand(key('w'),'editable',preferences,'darwin',true),'close-tab');
 assert.equal(workspaceKeyCommand(key('f'),'editable',preferences,'darwin',true),'find');
});
