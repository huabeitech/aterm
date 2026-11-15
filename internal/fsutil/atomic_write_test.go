package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFileCreatesFileWithMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config")

	if err := AtomicWriteFile(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("AtomicWriteFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("file content = %q, want hello", string(data))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %v, want 0600", got)
	}
}
