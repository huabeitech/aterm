package keys

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type memoryKeys struct {
	values   []BackupKey
	revision int
	fail     bool
}

func (m *memoryKeys) LoadKeys() ([]BackupKey, string, error) {
	return append([]BackupKey{}, m.values...), fmt.Sprint(m.revision), nil
}
func (m *memoryKeys) SaveKeys(revision string, values []BackupKey) error {
	if m.fail {
		return errors.New("disk full")
	}
	if revision != fmt.Sprint(m.revision) {
		return errors.New("stale revision")
	}
	m.values = append([]BackupKey{}, values...)
	m.revision++
	return nil
}
func keyFixture(t *testing.T) (*Store, *memoryKeys, string) {
	t.Helper()
	storage := &memoryKeys{}
	store := NewStore(storage)
	t.Cleanup(store.Close)
	return store, storage, ""
}
func TestGeneratedPrivateKeysStayOutOfCatalogAndCanBeUnlocked(t *testing.T) {
	store, secrets, _ := keyFixture(t)
	key, err := store.Create(CreateInput{Name: "Production", Algorithm: "ed25519", Passphrase: "correct horse battery"})
	if err != nil {
		t.Fatal(err)
	}
	if key.Algorithm != "ed25519" || !key.PassphraseProtected {
		t.Fatalf("key metadata: %#v", key)
	}
	bytes, err := json.Marshal(store.List())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), "PRIVATE KEY") || strings.Contains(string(bytes), "correct horse") {
		t.Fatal("private material leaked into public catalog")
	}
	private, err := store.ReadIdentity(Prefix + key.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(private)
	if _, err := ssh.ParsePrivateKey(private); err == nil {
		t.Fatal("passphrase protection was not applied")
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(private, []byte("correct horse battery"))
	if err != nil {
		t.Fatal(err)
	}
	if ssh.FingerprintSHA256(signer.PublicKey()) != key.Fingerprint {
		t.Fatal("catalog fingerprint does not match private key")
	}
	reopened := NewStore(secrets)
	defer reopened.Close()
	if len(reopened.List()) != 1 {
		t.Fatal("catalog did not survive reopening")
	}
	if _, err := store.Create(CreateInput{Name: "production", Algorithm: "ecdsa"}); err == nil {
		t.Fatal("case-insensitive duplicate name accepted")
	}

}
func TestImportRequiresAValidTokenAndRetainsItAfterIncorrectPassphrase(t *testing.T) {
	store, _, _ := keyFixture(t)
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "import", []byte("import passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	bytes := pem.EncodeToMemory(block)
	defer clear(bytes)
	preview, err := store.Preview("id_import.pem", bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.RequiresPassphrase || preview.SuggestedName != "id_import" {
		t.Fatalf("preview: %#v", preview)
	}
	if _, err := store.Import(preview.Token, "Imported", "wrong"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	key, err := store.Import(preview.Token, "Imported", "import passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if !key.PassphraseProtected {
		t.Fatal("import dropped original passphrase protection")
	}
	if _, err := store.Import(preview.Token, "Second", "import passphrase"); err == nil {
		t.Fatal("import token was reused")
	}
	preview, err = store.Preview("expires", bytes)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if _, err := store.Import(preview.Token, "Expired", "import passphrase"); err == nil {
		t.Fatal("expired import accepted")
	}
	public, _ := ssh.NewPublicKey(private.Public())
	if _, err := store.Preview("public.pub", ssh.MarshalAuthorizedKey(public)); err == nil {
		t.Fatal("public key imported as a private key")
	}
}
func TestFailedPersistencePreservesCatalogAndProtectedMaterial(t *testing.T) {
	store, secrets, _ := keyFixture(t)
	key, err := store.Create(CreateInput{Name: "Keep", Algorithm: "ecdsa"})
	if err != nil {
		t.Fatal(err)
	}
	original, err := json.Marshal(secrets.values)
	if err != nil {
		t.Fatal(err)
	}
	secrets.fail = true
	if _, err := store.Create(CreateInput{Name: "Fail", Algorithm: "ed25519"}); err == nil {
		t.Fatal("failed catalog write reported success")
	}
	if len(store.List()) != 1 || len(secrets.values) != 1 {
		t.Fatal("failed create changed key library")
	}
	if err := store.Delete(key.ID); err == nil {
		t.Fatal("failed delete reported success")
	}
	if _, err := store.ReadIdentity(Prefix + key.ID); err != nil {
		t.Fatal("failed delete lost protected private key")
	}
	after, _ := json.Marshal(secrets.values)
	if string(after) != string(original) {
		t.Fatal("failed mutation damaged original catalog")
	}
	secrets.fail = true
	if err := store.Delete(key.ID); err == nil {
		t.Fatal("denied system delete accepted")
	}
}
func TestPrivateExportRejectsSymlinksAndSetsOwnerOnlyPermissions(t *testing.T) {
	store, _, _ := keyFixture(t)
	key, err := store.Create(CreateInput{Name: "Export", Algorithm: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	destination := filepath.Join(directory, "id_export")
	if err := store.Export(key.ID, destination); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(destination)
	if info.Mode().Perm() != 0o600 {
		t.Fatal("export is not owner-only")
	}
	link := filepath.Join(directory, "symlink")
	if err := os.Symlink(destination, link); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(destination)
	if err := WritePrivateExport(link, []byte("overwrite")); err == nil {
		t.Fatal("symlink export accepted")
	}
	after, _ := os.ReadFile(destination)
	if string(before) != string(after) {
		t.Fatal("export followed a symlink")
	}
}
func TestAddToAgentUnlocksProtectedKeyWithoutExportingATemporaryFile(t *testing.T) {
	store, _, _ := keyFixture(t)
	key, err := store.Create(CreateInput{Name: "Agent", Algorithm: "ed25519", Passphrase: "agent passphrase"})
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp("", "ak-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	socket := filepath.Join(directory, "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ring := agent.NewKeyring()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { defer connection.Close(); _ = agent.ServeAgent(ring, connection) }()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.AddToAgent(ctx, key.ID, "wrong", socket); err == nil {
		t.Fatal("wrong agent passphrase accepted")
	}
	if err := store.AddToAgent(ctx, key.ID, "agent passphrase", socket); err != nil {
		t.Fatal(err)
	}
	keys, err := ring.List()
	if err != nil || len(keys) != 1 {
		t.Fatalf("agent keys %v: %v", keys, err)
	}
	public, err := ssh.ParsePublicKey(keys[0].Blob)
	if err != nil || ssh.FingerprintSHA256(public) != key.Fingerprint {
		t.Fatal("agent received the wrong key")
	}
}

func TestProtectedBackupRoundTripAndFailedRestoreRollback(t *testing.T) {
	source, _, _ := keyFixture(t)
	key, err := source.Create(CreateInput{Name: "Backup", Algorithm: "ed25519", Passphrase: "backup passphrase"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := source.Backup()
	if err != nil {
		t.Fatal(err)
	}
	destination, secrets, _ := keyFixture(t)
	original, err := destination.Create(CreateInput{Name: "Original", Algorithm: "ecdsa"})
	if err != nil {
		t.Fatal(err)
	}
	oldBytes, err := destination.ReadIdentity(Prefix + original.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(oldBytes)
	secrets.fail = true
	if err := destination.RestoreBackup(payload); err == nil {
		t.Fatal("failed restore reported success")
	}
	retained, err := destination.ReadIdentity(Prefix + original.ID)
	if err != nil || string(retained) != string(oldBytes) {
		t.Fatal("restore rollback lost original protected key")
	}
	if len(secrets.values) != 1 || len(destination.List()) != 1 {
		t.Fatal("failed restore left partial protected state")
	}
	secrets.fail = false
	reopened := NewStore(secrets)
	defer reopened.Close()
	if err := reopened.RestoreBackup(payload); err != nil {
		t.Fatal(err)
	}
	material, err := reopened.ReadIdentity(Prefix + key.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(material)
	signer, err := ssh.ParsePrivateKeyWithPassphrase(material, []byte("backup passphrase"))
	if err != nil || ssh.FingerprintSHA256(signer.PublicKey()) != key.Fingerprint {
		t.Fatal("restored encrypted key is not usable")
	}
	if _, err := reopened.ReadIdentity(Prefix + original.ID); err == nil {
		t.Fatal("successful restore retained a replaced key")
	}
	payload[0].Fingerprint = "SHA256:invalid"
	if err := reopened.RestoreBackup(payload); err == nil {
		t.Fatal("inconsistent key backup accepted")
	}
}

func TestDeniedSecureStorageCannotCreateAKey(t *testing.T) {
	store, secrets, _ := keyFixture(t)
	secrets.fail = true
	if _, err := store.Create(CreateInput{Name: "Denied", Algorithm: "ed25519"}); err == nil {
		t.Fatal("denied system storage accepted a key")
	}
	if len(store.List()) != 0 {
		t.Fatal("denied storage changed public catalog")
	}
	if len(secrets.values) != 0 {
		t.Fatal("denied storage persisted metadata without its private key")
	}
}

func TestGeneratedRSAKeyIs4096BitAndOpenSSHCompatible(t *testing.T) {
	tool, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("OpenSSH keygen is unavailable")
	}
	store, _, _ := keyFixture(t)
	key, err := store.Create(CreateInput{Name: "RSA", Algorithm: "rsa"})
	if err != nil {
		t.Fatal(err)
	}
	public, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	cryptoKey, ok := public.(ssh.CryptoPublicKey)
	if !ok {
		t.Fatal("RSA public key does not expose its crypto key")
	}
	rsaKey, ok := cryptoKey.CryptoPublicKey().(*rsa.PublicKey)
	if !ok || rsaKey.N.BitLen() != 4096 {
		t.Fatal("generated RSA key is not 4096 bit")
	}
	name := filepath.Join(t.TempDir(), "id_rsa")
	if err := store.Export(key.ID, name); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(tool, "-y", "-f", name).Output()
	if err != nil {
		t.Fatalf("OpenSSH could not read the generated key: %v", err)
	}
	checked, _, _, _, err := ssh.ParseAuthorizedKey(output)
	if err != nil || ssh.FingerprintSHA256(checked) != key.Fingerprint {
		t.Fatal("OpenSSH derived a different public key")
	}
}

func TestClosedKeyStoreCannotPersistNewCredentials(t *testing.T) {
	store, secrets, _ := keyFixture(t)
	store.Close()
	if _, err := store.Create(CreateInput{Name: "After shutdown", Algorithm: "ed25519"}); err == nil {
		t.Fatal("closed store generated a new key")
	}
	if len(secrets.values) != 0 {
		t.Fatal("closed store wrote protected credentials")
	}
	if _, err := store.Backup(); err == nil {
		t.Fatal("closed store exported private material")
	}
}

func TestIdentityNameUsesPublicCatalogWithoutUnlockingSecrets(t *testing.T) {
	store, secrets, _ := keyFixture(t)
	key, err := store.Create(CreateInput{Name: "Production Key", Algorithm: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	reference := Prefix + key.ID
	secrets.values[0].PrivateKey = "" // Public metadata never exposes private material.
	if name := store.IdentityName(reference); name != "Production Key" {
		t.Fatal(name)
	}
	for _, path := range []string{"/fixture/id_ed25519", Prefix + "deleted"} {
		if name := store.IdentityName(path); name != path {
			t.Fatal("unexpected metadata fallback", name)
		}
	}
}
