package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestElectronPluginPackageInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Electron package validation")
	}
	directory := t.TempDir()
	script := fmt.Sprintf(`import {writeFileSync,readFileSync} from 'node:fs';
import {join} from 'node:path';
import {inspectDirectory} from %q;
import {createArchive,inspectArchive} from %q;
import {executionPolicyDigest,pluginGrantId} from %q;
const root=process.argv[2],out=process.argv[3];
if(process.argv[4]==='verify') { await inspectArchive(join(out,'go.aterm-plugin')); process.exit(0); }
for (const name of ['resource-monitor','docker']) {
 const plugin=inspectDirectory(join(root,'plugins',name,'dist'));
 writeFileSync(join(out,name+'.aterm-plugin'),createArchive(plugin));
 writeFileSync(join(out,name+'.json'),JSON.stringify({plugin,policy:executionPolicyDigest(plugin.manifest),grant:pluginGrantId({id:'host-id',hostName:'example.test',port:22,user:'fixture',jumpHostID:'jump-id',proxyJump:''},plugin.manifest)}));
}
`, filepath.Join(reference, "packages/plugin-format/src/directory.ts"), filepath.Join(reference, "packages/plugin-format/src/archive.ts"), filepath.Join(reference, "src/main/plugins/permissions.ts"))
	scriptPath := filepath.Join(directory, "interop.mts")
	os.WriteFile(scriptPath, []byte(script), 0600)
	run := func(mode string) {
		t.Helper()
		command := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), scriptPath, reference, directory, mode)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("Electron codec: %v\n%s", err, output)
		}
	}
	run("export")
	for _, name := range []string{"resource-monitor", "docker"} {
		plugin, err := InspectArchive(filepath.Join(directory, name+".aterm-plugin"))
		if err != nil {
			t.Fatal(name, err)
		}
		data, _ := os.ReadFile(filepath.Join(directory, name+".json"))
		var expected struct {
			Plugin json.RawMessage `json:"plugin"`
			Policy string          `json:"policy"`
			Grant  string          `json:"grant"`
		}
		json.Unmarshal(data, &expected)
		var actualValue, expectedValue any
		encoded, _ := marshalJSON(plugin)
		json.Unmarshal(encoded, &actualValue)
		json.Unmarshal(expected.Plugin, &expectedValue)
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Fatalf("normalized %s package differs from Electron", name)
		}
		if PolicyDigest(plugin.Manifest) != expected.Policy {
			t.Fatal(name, "execution policy digest differs")
		}
		if GrantID(GrantTarget{ID: "host-id", HostName: "example.test", Port: 22, User: "fixture", JumpHostID: "jump-id"}, plugin.Manifest) != expected.Grant {
			t.Fatal(name, "host grant identity differs")
		}
		store, _ := NewPackageStore(filepath.Join(directory, "installed", name))
		key, err := store.Install(plugin)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := store.Load(key)
		if err != nil {
			t.Fatal(err)
		}
		files, _ := packageFiles(loaded)
		os.WriteFile(filepath.Join(directory, "go.aterm-plugin"), zipFiles(t, files), 0600)
		run("verify")
	}
}

func TestElectronCanonicalLocaleInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Intl parity validation")
	}
	directory := t.TempDir()
	scriptPath := filepath.Join(directory, "locales.mjs")
	script := `import {writeFileSync} from 'node:fs';
 const locales=[]; const alphabet='abcdefghijklmnopqrstuvwxyz';
 for (const a of alphabet) for (const b of alphabet) {locales.push(a+b);for(const c of alphabet) locales.push(a+b+c);}
 for (const suffix of ['Latn','Cyrl','Qaai','Qaaa','US','BU','SU','ZZ','QQ','123','1901','abcde','arevela','arevmda','baku1926','colb1945','heploc','luna1918','petr1708','aaland','polytoni','bokmal','nynorsk','saaho','hakka','xiang','lojban']) for (const language of ['en','xx','hy','und','zh']) locales.push(language+'-'+suffix);
 locales.push('sl-biske-rozaj','sl-rozaj-biske','xx-Latn-YY-abcde','en-abcde-abcde');
 writeFileSync(process.argv[2],JSON.stringify(locales.map(locale=>{try{return {locale,valid:Intl.getCanonicalLocales(locale)[0]===locale}}catch{return {locale,valid:false}}})));`
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "expected.json")
	command := exec.Command("node", scriptPath, path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Intl: %v %s", err, output)
	}
	data, _ := os.ReadFile(path)
	var cases []struct {
		Locale string `json:"locale"`
		Valid  bool   `json:"valid"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	mismatches := 0
	for _, item := range cases {
		if valid := canonicalLocale(item.Locale); valid != item.Valid {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("%s: Go acceptance=%v; Intl=%v", item.Locale, valid, item.Valid)
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d locale mismatches", mismatches)
	}
}
