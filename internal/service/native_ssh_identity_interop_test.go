package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestElectronNativeIdentityLoadingInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for actual Electron key-loader oracle")
	}
	type vector struct {
		Directory  string `json:"directory"`
		Path       string `json:"path"`
		Explicit   bool   `json:"explicit"`
		Managed    bool   `json:"managed"`
		Material   []byte `json:"material"`
		Passphrase string `json:"passphrase"`
	}
	encrypted := encryptedIdentityFixture(t)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "disposable oracle")
	if err != nil {
		t.Fatal(err)
	}
	plain := pem.EncodeToMemory(block)
	var vectors []vector
	for _, explicit := range []bool{false, true} {
		for _, kind := range []string{"missing", "malformed", "oversized", "plain", "encrypted-correct", "encrypted-wrong"} {
			directory := t.TempDir()
			path := filepath.Join(directory, "id_ed25519")
			passphrase := "correct"
			var material []byte
			switch kind {
			case "malformed":
				material = []byte("not a key")
			case "oversized":
				material = make([]byte, maxNativeIdentityBytes+1)
			case "plain":
				material = plain
			case "encrypted-correct":
				material = encrypted
			case "encrypted-wrong":
				material = encrypted
				passphrase = "incorrect"
			}
			if kind != "missing" {
				if err := os.WriteFile(path, material, 0600); err != nil {
					t.Fatal(err)
				}
			}
			vectors = append(vectors, vector{Directory: directory, Path: path, Explicit: explicit, Passphrase: passphrase})
		}
	}
	for _, kind := range []string{"plain", "malformed", "encrypted-correct", "encrypted-wrong"} {
		material := plain
		passphrase := "correct"
		switch kind {
		case "malformed":
			material = []byte("not a key")
		case "encrypted-correct":
			material = encrypted
		case "encrypted-wrong":
			material = encrypted
			passphrase = "incorrect"
		}
		vectors = append(vectors, vector{Directory: t.TempDir(), Path: "aterm-key://fixture", Explicit: true, Managed: true, Material: material, Passphrase: passphrase})
	}
	data, err := json.Marshal(vectors)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "vectors.json")
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("testdata/electron-identity-oracle.mts")
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), script, reference, input).CombinedOutput()
	if err != nil {
		t.Fatalf("Electron identity oracle: %v: %s", err, output)
	}
	type promptRecord struct {
		Title   string            `json:"title"`
		Message string            `json:"message"`
		Fields  []CredentialField `json:"fields"`
	}
	type result struct {
		Offered bool           `json:"offered"`
		Error   string         `json:"error"`
		Prompts []promptRecord `json:"prompts"`
	}

	var expected []result
	if err := json.Unmarshal(output, &expected); err != nil {
		t.Fatalf("invalid oracle output: %v: %s", err, output)
	}
	if len(expected) != len(vectors) {
		t.Fatal("invalid oracle count")
	}
	for index, vector := range vectors {
		got := result{Prompts: []promptRecord{}}
		identity, err := loadNativeSSHIdentity(context.Background(), vector.Path, vector.Explicit, NativeSSHAuthOptions{
			ReadIdentity: func(string) ([]byte, error) { return append([]byte(nil), vector.Material...), nil },
			IdentityName: func(string) string { return "Fixture Key" },
			Prompt: func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
				got.Prompts = append(got.Prompts, promptRecord{prompt.Title, prompt.Message, prompt.Fields})
				return []string{vector.Passphrase}, nil
			},
		})
		got.Offered = identity != nil
		if err != nil {
			got.Error = err.Error()
		}
		if !reflect.DeepEqual(got, expected[index]) {
			t.Errorf("vector %d differs: Go=%+v Electron=%+v", index, got, expected[index])
		}
	}
	t.Logf("compared %d actual Electron key-loading cases", len(vectors))
}
