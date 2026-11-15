//go:build !windows && !cgo

package localpty

import (
	"strings"
	"testing"
)

func TestLocalPasswdLookupUsesEffectiveUID(t *testing.T) {
	input := "root:x:0:0:root:/root:/bin/sh\nother:x:1001:1001:other:/home/other:/bin/bash\nfixture:x:42:42:fixture:/home/fixture:/custom/shell\n"
	if got, err := readPasswdShell(strings.NewReader(input), 42); err != nil || got != "/custom/shell" {
		t.Fatal(got, err)
	}
	if _, err := readPasswdShell(strings.NewReader(input), 99); err == nil {
		t.Fatal("missing account silently accepted")
	}
	if got, err := readPasswdShell(strings.NewReader("fixture:x:42:42:fixture:/home/fixture:\n"), 42); err != nil || got != "" {
		t.Fatal(got, err)
	}
}
