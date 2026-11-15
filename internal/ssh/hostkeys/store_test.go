package hostkeys

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func fingerprint(marker byte) string {
	return "SHA256:" + base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat(string(marker), 32)))
}
func TestTrustReloadReplacementAndStaleApproval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-host-keys.json")
	store := New(path)
	ctx := context.Background()
	first, second, third := fingerprint('a'), fingerprint('b'), fingerprint('c')
	if err := store.Commit(ctx, "Example.test", "22", "", first); err != nil {
		t.Fatal(err)
	}
	if got, err := New(path).Lookup("example.test", "22"); err != nil || got != first {
		t.Fatal(got, err)
	}
	if err := store.Commit(ctx, "example.test", "22", first, second); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := store.Commit(ctx, "example.test", "22", first, third); !errors.Is(err, ErrChanged) {
		t.Fatalf("stale approval: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Commit(canceled, "example.test", "22", second, third); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("rejected approval changed trust file")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("trust file permissions", info.Mode())
	}
}
func TestConcurrentStoresPreserveIndependentHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-host-keys.json")
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			if err := New(path).Commit(context.Background(), fmt.Sprintf("host-%d", i), "22", "", fingerprint('a')); err != nil {
				t.Error(err)
			}
		}(i)
	}
	group.Wait()
	for i := 0; i < 20; i++ {
		if got, err := New(path).Lookup(fmt.Sprintf("host-%d", i), "22"); err != nil || got != fingerprint('a') {
			t.Fatal("lost concurrent trust update", i, got, err)
		}
	}
}
func TestCorruptTrustFilesArePreserved(t *testing.T) {
	for _, data := range []string{`null`, `{"version":2,"keys":{}}`, `{"version":1,"keys":null}`, `{"version":1,"keys":{"host:22":"bad"}}`, `{"version":1,"version":1,"keys":{}}`, fmt.Sprintf(`{"version":1,"keys":{"host:22":%q,"host:22":%q}}`, fingerprint('a'), fingerprint('b')), strings.Repeat(" ", maxBytes+1)} {
		path := filepath.Join(t.TempDir(), "ssh-host-keys.json")
		os.WriteFile(path, []byte(data), 0600)
		if _, err := New(path).Lookup("host", "22"); err == nil {
			t.Fatal("invalid trust file accepted")
		}
		if err := New(path).Commit(context.Background(), "host", "22", "", fingerprint('a')); err == nil {
			t.Fatal("invalid trust file overwritten")
		}
		after, _ := os.ReadFile(path)
		if string(after) != data {
			t.Fatal("corrupt trust file changed")
		}
	}
}
