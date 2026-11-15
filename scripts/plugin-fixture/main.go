// Generates disposable, self-authored packages for native plugin transport QA.
package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sandboxProbe = `(async () => {
 if (typeof document !== 'undefined' || typeof Worker !== 'undefined' || typeof SharedWorker !== 'undefined' || typeof window.parent !== 'undefined' || typeof globalThis._wails !== 'undefined') throw Error('Unisolated worker');
 let denied = false;
 try { await fetch('data:text/plain,blocked'); } catch { denied = true; }
 if (!denied) throw Error('Worker connection policy is missing');
 const api = window.atermPlugin;
 async function publish(token, prefix, increment) {
  const previous = await api.storage.get('sample');
  const count = (typeof previous === 'number' ? previous : 0) + increment;
  await api.storage.set('sample', count);
  await api.request('status.publish', {token, state:'ready', items:[{id:'sample',label:'Parity',value:prefix+' '+count,command:'details'}],data:{count}});
  return {count};
 }
 api.commands.registerCommand('increment', ({token}) => publish(token,'Command',100));
 api.onRefresh(({token}) => publish(token,'Sample',1));
 await api.request('runtime.ready', {});
})().catch(() => { throw Error('Sandbox probe failed'); });`

const detailView = `<!doctype html><html><head><style>body{font:13px system-ui;color:var(--mac-text);background:var(--workspace-background);padding:16px}button{font:inherit;margin:6px 6px 6px 0}</style></head><body>
<h1>Sandbox Details</h1><p id="isolation">Checking isolation</p><p id="sample">Waiting for state</p><p id="command">No command yet</p><p id="navigation">No navigation attempt</p>
<button id="increment">Increment sample</button><button id="navigate">Test navigation boundary</button><button id="close">Close view</button>
<script>
(async()=>{
 const api=window.atermView;await api.ready;
 let parentDenied=false,networkDenied=false;
 try{void parent.document.body}catch{parentDenied=true}
 try{await fetch('data:text/plain,blocked')}catch{networkDenied=true}
 if (window.webkit?.messageHandlers?.external) window.webkit.messageHandlers.external.postMessage('aterm-plugin-native-probe');
 const isolated=parentDenied&&networkDenied&&typeof window._wails==='undefined'&&typeof Worker==='undefined'&&typeof RTCPeerConnection==='undefined';
 document.getElementById('isolation').textContent=isolated?'Isolation verified':'Isolation failed';
 function render(state){document.getElementById('sample').textContent='Sample '+state.status?.data?.count}
 api.onDidChange(render);render(api.state);
 document.getElementById('increment').onclick=async()=>{const value=await api.executeCommand('increment');document.getElementById('command').textContent='Command returned '+value.count};
 document.getElementById('navigate').onclick=()=>{location.href='wails://localhost/';setTimeout(()=>document.getElementById('navigation').textContent='Navigation blocked',100)};
 document.getElementById('close').onclick=()=>api.close();
})().catch(()=>{document.getElementById('isolation').textContent='View bootstrap failed'});
</script></body></html>`

func main() {
	destination := flag.String("out", "", "new temporary output directory")
	flag.Parse()
	directory := *destination
	if directory == "" {
		var err error
		directory, err = os.MkdirTemp("", "aterm-plugin-fixture-*")
		must(err)
	} else {
		absolute, err := filepath.Abs(directory)
		must(err)
		temporary, err := filepath.EvalSymlinks(os.TempDir())
		must(err)
		parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
		must(err)
		directory = filepath.Join(parent, filepath.Base(absolute))
		relative, err := filepath.Rel(temporary, directory)
		must(err)
		if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			panic("fixture output must be inside the temporary directory")
		}
		must(os.Mkdir(directory, 0700))
	}
	for _, fixture := range []struct{ name, title, source string }{{"sandbox-probe", "Sandbox Probe", sandboxProbe}, {"invalid-syntax", "Invalid Syntax", "function { invalid"}} {
		id := "example." + fixture.name
		manifest := map[string]any{"schemaVersion": 1, "id": id, "name": fixture.title, "version": "1.1.0", "runtime": "sandbox", "entry": "plugin.js", "permissions": []string{}, "contributes": map[string]any{"terminalStatusBar": map[string]string{"id": id + ".status"}, "commands": []map[string]string{{"id": "increment", "title": "Increment sample"}}}}
		if fixture.name == "sandbox-probe" {
			manifest["contributes"] = map[string]any{"terminalStatusBar": map[string]string{"id": id + ".status"}, "commands": []map[string]any{{"id": "increment", "title": "Increment sample"}, {"id": "details", "title": "Open details", "action": map[string]string{"type": "view.open", "view": "details"}}}, "views": []map[string]string{{"id": "details", "title": "Sandbox Details", "entry": "ui/details.html", "placement": "terminal.sidePanel"}}}
		}
		path := filepath.Join(directory, fixture.name+".aterm-plugin")
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		must(err)
		writer := zip.NewWriter(file)
		entry, err := writer.Create("manifest.json")
		must(err)
		data, err := json.Marshal(manifest)
		must(err)
		_, err = entry.Write(data)
		must(err)
		entry, err = writer.Create("plugin.js")
		must(err)
		_, err = entry.Write([]byte(fixture.source))
		must(err)
		if fixture.name == "sandbox-probe" {
			entry, err = writer.Create("ui/details.html")
			must(err)
			_, err = entry.Write([]byte(detailView))
			must(err)
		}
		must(writer.Close())
		must(file.Close())
	}
	fmt.Println(directory)
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
