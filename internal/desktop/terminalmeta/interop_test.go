package terminalmeta

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func TestElectronDirectoryMetadataContract(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR to compare Electron metadata source")
	}
	input := "p12\x00\nn/work space 中文\x00\np13\x00\nn/path\nwith newline\x00"
	script := `
const fs=require('node:fs'),path=require('node:path');
const [root,input]=process.argv.slice(1),ts=require(path.join(root,'node_modules/typescript'));
function load(name){const m={exports:{}};const source=fs.readFileSync(path.join(root,'src/main/terminal',name),'utf8');const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;Function('module','exports','require',compiled)(m,m.exports,require);return m.exports}
const local=load('localDirectory.ts'),remote=load('remoteDirectory.ts');
process.stdout.write(JSON.stringify({command:remote.remoteDirectoryCommand,paths:Object.fromEntries(local.parseWorkingDirectories(Buffer.from(input,'base64').toString('utf8')))}));
`
	output, err := exec.Command("node", "-e", script, reference, base64.StdEncoding.EncodeToString([]byte(input))).CombinedOutput()
	if err != nil {
		t.Fatalf("Electron metadata source: %v: %s", err, output)
	}
	var result struct {
		Command string         `json:"command"`
		Paths   map[int]string `json:"paths"`
	}
	if err = json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	if result.Command != RemoteDirectoryCommand {
		t.Fatal("remote directory probe differs from Electron source")
	}
	if !reflect.DeepEqual(result.Paths, ParseWorkingDirectories(input)) {
		t.Fatal("local directory parser differs from Electron source")
	}
}
