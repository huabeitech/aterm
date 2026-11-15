package localpty

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func directoryReport(nonce, path string) string {
	return "\x1b]777;aterm-cwd;" + nonce + ";" + base64.StdEncoding.EncodeToString([]byte(path)) + "\x07"
}
func directoryReportVectors() [][][]byte {
	var vectors [][][]byte
	frame := "before" + directoryReport("private", "C:\\space 中文") + directoryReport("private", `\\server\share\folder`) + "after"
	for split := 0; split <= len(frame); split++ {
		vectors = append(vectors, [][]byte{[]byte(frame[:split]), []byte(frame[split:])})
	}
	var singleBytes [][]byte
	for _, value := range []byte(frame) {
		singleBytes = append(singleBytes, []byte{value})
	}
	vectors = append(vectors, singleBytes)
	for _, path := range []string{`C:relative`, `relative`, `\rooted`, `/rooted`, `C:/valid`, "C:\\bad\npath", "C:\\bad\x00path", "C:\\bad\x7fpath", "C:\\" + string([]byte{0xe4, 0xb8}), "C:\\" + string([]byte{0xed, 0xa0, 0x80})} {
		vectors = append(vectors, [][]byte{[]byte(directoryReport("private", path))})
	}
	for _, frame := range []string{
		directoryReport("wrong", `C:\wrong`),
		"\x1b]777;aterm-cwd;private;%%%\x07",
		"\x1b]777;aterm-cwd;private;QzovYQ\x07",  // unpadded C:/a
		"\x1b]777;aterm-cwd;private;QzovYQA\x07", // control byte
		"\x1b]777;aterm-cwd;private;QzovYQx\x07", // dangling group, same Node decoding
		"\x1b]777;aterm-cwd;private;" + strings.Repeat("a", 12001) + "\x07",
		"\x1b]777;aterm-cwd;private;" + strings.Repeat("a", 17000) + directoryReport("private", `C:\recovered`),
	} {
		vectors = append(vectors, [][]byte{[]byte(frame)})
	}
	return vectors
}
func TestDirectoryReportsAreBoundedAndRecoverAfterInvalidFrames(t *testing.T) {
	var got []string
	parser := NewDirectoryReports("private", func(path string) { got = append(got, path) })
	parser.Consume([]byte(strings.Repeat("x", 1024*1024)))
	if len(parser.pending) > len(parser.prefix) {
		t.Fatal("ordinary output retained")
	}
	parser.Consume([]byte(parser.prefix + strings.Repeat("a", 1024*1024)))
	if len(parser.pending) > 16384 {
		t.Fatal("unterminated frame retained")
	}
	frame := directoryReport("private", `C:\space 中文`)
	for _, value := range []byte(frame) {
		parser.Consume([]byte{value})
	}
	if !reflect.DeepEqual(got, []string{`C:\space 中文`}) {
		t.Fatal(got)
	}
}
func TestPowerShellArgsLimitToSupportedShells(t *testing.T) {
	for _, shell := range []string{"pwsh", "powershell.exe", `C:\Windows\PowerShell.EXE`, "/fixture/pwsh"} {
		args := PowerShellIntegration(shell, "private")
		if len(args) != 4 || args[2] != "-EncodedCommand" {
			t.Fatal(shell, args)
		}
	}
	for _, shell := range []string{"cmd.exe", "bash", "notpowershell.exe", "pwsh.exe.bak"} {
		if len(PowerShellIntegration(shell, "private")) != 0 {
			t.Fatal(shell)
		}
	}
}
func TestElectronPowerShellProtocolInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Electron PowerShell oracle")
	}
	directory := t.TempDir()
	script := filepath.Join(directory, "powershell.mts")
	code := `import {readFileSync} from 'node:fs';import {pathToFileURL} from 'node:url';const {powerShellIntegration,WindowsDirectoryReports}=await import(pathToFileURL(process.argv[2]).href);const input=JSON.parse(readFileSync(process.argv[3],'utf8'));process.stdout.write(JSON.stringify({args:input.shells.map(s=>powerShellIntegration(s,'private')),reports:input.vectors.map(chunks=>{const paths=[];const parser=new WindowsDirectoryReports('private',p=>paths.push(p));for(const chunk of chunks)parser.consume(Buffer.from(chunk,'base64'));return paths})}));`
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	shells := []string{"pwsh", "powershell.exe", `C:\Windows\PowerShell.EXE`, "/fixture/pwsh", "cmd.exe", "bash", "notpowershell.exe", "pwsh.exe.bak"}
	vectors := directoryReportVectors()
	data, err := json.Marshal(struct {
		Shells  []string   `json:"shells"`
		Vectors [][][]byte `json:"vectors"`
	}{shells, vectors})
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(directory, "vectors.json")
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), script, filepath.Join(reference, "src/main/terminal/windowsShell.ts"), input).CombinedOutput()
	if err != nil {
		t.Fatalf("Electron PowerShell oracle: %v: %s", err, output)
	}
	var expected struct {
		Args    [][]string `json:"args"`
		Reports [][]string `json:"reports"`
	}
	if err := json.Unmarshal(output, &expected); err != nil {
		t.Fatal(err)
	}
	if len(expected.Args) != len(shells) || len(expected.Reports) != len(vectors) {
		t.Fatal("invalid oracle lengths")
	}
	for index, shell := range shells {
		got := PowerShellIntegration(shell, "private")
		if len(got) != 0 && !reflect.DeepEqual(got, expected.Args[index]) || len(got) != len(expected.Args[index]) {
			t.Errorf("shell %s args differ", shell)
		}
	}
	for index, chunks := range vectors {
		got := []string{}
		parser := NewDirectoryReports("private", func(path string) { got = append(got, path) })
		for _, chunk := range chunks {
			parser.Consume(chunk)
		}
		if !reflect.DeepEqual(got, expected.Reports[index]) {
			t.Errorf("report vector %d: Go=%q Electron=%q", index, got, expected.Reports[index])
		}
	}
	t.Logf("matched %d shell launches and %d chunked report vectors", len(shells), len(vectors))
}
