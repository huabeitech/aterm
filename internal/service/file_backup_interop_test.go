package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Opt-in because parity validation needs the user's Electron reference checkout.
// Both runtimes execute their real codecs; no reimplementation is used as oracle.
func TestElectronEncryptedBackupInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR to validate the Electron codec")
	}
	_, goBackup := makeFileBackup(t)
	directory := t.TempDir()
	electronBackup := filepath.Join(directory, "electron.enc")
	script := fmt.Sprintf(`import { readFileSync, writeFileSync } from 'node:fs';
import { unlockCompatibleVault, sealCompatibleVault } from %q;
const opened = await unlockCompatibleVault(readFileSync(process.argv[2]), 'backup-fixture-password');
try {
 if (opened.payload.hostCount !== 1 || opened.payload.managedKeys?.length !== 1 || !opened.payload.managedKeys[0].privateKey.includes('PRIVATE KEY')) throw Error('Go workspace lost managed key or host metadata');
 const encoded = sealCompatibleVault(readFileSync(process.argv[2]), opened.key, {...opened.payload, sourceDevice:'Electron interop fixture'});
 writeFileSync(process.argv[3],encoded,{mode:0o600});
} finally { opened.key.fill(0); }
`, filepath.Join(reference, "src/main/storage/compatibleVault.ts"))
	scriptPath := filepath.Join(directory, "interop.mts")
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), scriptPath, goBackup, electronBackup)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Electron codec: %v\n%s", err, output)
	}
	destination, keys, store := managedBackupService(t)
	confirmed := false
	_, err = destination.RestoreEncryptedBackup(context.Background(), electronBackup, "backup-fixture-password", func(meta SyncMetadata) bool {
		confirmed = meta.SourceDeviceName == "Electron interop fixture"
		return confirmed
	})
	if err != nil || !confirmed {
		t.Fatal("could not restore Electron-sealed workspace", err)
	}
	hosts, _ := store.ListHosts()
	if len(hosts) != 1 || len(keys.List()) != 1 {
		t.Fatal("Electron round trip lost records")
	}
	if _, err := keys.ReadIdentity(hosts[0].IdentityFile); err != nil {
		t.Fatal(err)
	}
}
