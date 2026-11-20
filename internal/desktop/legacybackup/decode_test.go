package legacybackup

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"golang.org/x/crypto/scrypt"
)

const fixturePassword = "legacy-fixture-password"

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/electron-aes-v1.enc")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func TestDecodeActualElectronAESLibraryBackup(t *testing.T) {
	data := fixture(t)
	if !Recognizes(data) {
		t.Fatal("Electron backup was not recognized")
	}
	workspace, err := Decode(data, fixturePassword)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Hosts) != 2 || len(workspace.Directories) != 2 || len(workspace.SortOrders) < 4 {
		t.Fatal("library metadata lost", workspace)
	}
	byAlias := map[string]int{}
	for i, h := range workspace.Hosts {
		byAlias[h.Alias] = i
		if h.IdentityFile != "/fixture/id_ed25519" || !h.Compression || !h.ForwardAgent || h.Port != 2222 {
			t.Fatal("legacy connection options changed", h)
		}
	}
	target := workspace.Hosts[byAlias["fixture-target"]]
	jump := workspace.Hosts[byAlias["fixture-jump"]]
	if target.JumpHostID != jump.ID || target.ProxyJump != jump.Alias || workspace.Passwords[target.ID] != "fixture-secret-中文" {
		t.Fatal("jump host or password lost")
	}
	if workspace.Preferences["appFontScale"] != float64(110) || workspace.Preferences["terminalFontSize"] != float64(18) || workspace.Preferences["terminalWordNavigation"] != "terminal" {
		t.Fatal("preferences lost", workspace.Preferences)
	}
	if _, err = Decode(data, "incorrect"); !errors.Is(err, ErrPassword) {
		t.Fatal("wrong password accepted", err)
	}
	var damaged envelope
	_ = json.Unmarshal(data, &damaged)
	ciphertext, _ := decodeField(damaged.Data)
	ciphertext[0] ^= 1
	damaged.Data = base64.StdEncoding.EncodeToString(ciphertext)
	data, _ = json.Marshal(damaged)
	if _, err = Decode(data, fixturePassword); !errors.Is(err, ErrPassword) {
		t.Fatal("damaged ciphertext accepted", err)
	}
}

// Mutation fixtures remain authenticated, so validation is exercised after
// decryption rather than merely proving that ciphertext tampering fails.
func modifiedFixture(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	var outer envelope
	_ = json.Unmarshal(fixture(t), &outer)
	salt, _ := decodeField(outer.Salt)
	key, err := scrypt.Key([]byte(fixturePassword), salt, 16384, 8, 1, 32)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	plaintext, err := open(outer.sealed, key, "aterm-backup-v1")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plaintext)
	var content map[string]any
	if err = json.Unmarshal(plaintext, &content); err != nil {
		t.Fatal(err)
	}
	change(content)
	plaintext, err = json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plaintext)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	iv, _ := decodeField(outer.IV)
	sealed := gcm.Seal(nil, iv, plaintext, []byte("aterm-backup-v1"))
	outer.Data = base64.StdEncoding.EncodeToString(sealed[:len(sealed)-16])
	outer.Tag = base64.StdEncoding.EncodeToString(sealed[len(sealed)-16:])
	data, err := json.Marshal(outer)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func TestRejectAuthenticatedInvalidLibraries(t *testing.T) {
	cases := map[string]func(map[string]any){
		"missing preferences": func(v map[string]any) { delete(v, "preferences") },
		"invalid preferences": func(v map[string]any) { v["preferences"].(map[string]any)["terminalFontSize"] = 1000 },
		"missing host field": func(v map[string]any) {
			delete(v["library"].(map[string]any)["hosts"].([]any)[0].(map[string]any), "port")
		},
		"null flag": func(v map[string]any) {
			v["library"].(map[string]any)["hosts"].([]any)[0].(map[string]any)["favorite"] = nil
		},
		"folder cycle": func(v map[string]any) {
			d := v["library"].(map[string]any)["directories"].([]any)[0].(map[string]any)
			d["parentID"] = d["id"]
		},
		"jump cycle": func(v map[string]any) {
			h := v["library"].(map[string]any)["hosts"].([]any)[0].(map[string]any)
			h["jumpHostID"] = h["id"]
		},
		"duplicate host": func(v map[string]any) {
			lib := v["library"].(map[string]any)
			hosts := lib["hosts"].([]any)
			lib["hosts"] = append(hosts, hosts[0])
		},
		"missing folder": func(v map[string]any) {
			v["library"].(map[string]any)["hosts"].([]any)[0].(map[string]any)["directoryID"] = "missing"
		},
		"invalid sort": func(v map[string]any) {
			v["library"].(map[string]any)["hosts"].([]any)[0].(map[string]any)["sortOrders"] = map[string]any{"invalid:": 0}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(modifiedFixture(t, change), fixturePassword); err == nil {
				t.Fatal("malformed library accepted")
			}
		})
	}
}
