//go:build !windows

package localpty

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestUnixShellSelectionOrder(t *testing.T) {
	for _, vector := range []struct{ account, environment, want string }{
		{"/bin/zsh", "/bin/bash", "/bin/zsh"},
		{"/custom/shell with spaces", "/bin/sh", "/custom/shell with spaces"},
		{"/bin/sh", "/missing/shell", "/bin/sh"},
		{"", "/bin/bash", "/bin/bash"},
		{"", "", "/bin/sh"},
		{"", " ", " "},
	} {
		if got := selectUnixShell(vector.account, vector.environment); got != vector.want {
			t.Errorf("account=%q environment=%q: %q", vector.account, vector.environment, got)
		}
	}
}
func TestLoginShellMatchesNodeEffectiveUser(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node runtime not available for effective-user oracle")
	}
	for _, environment := range []string{"", "/definitely-not-an-installed-shell", "/bin/sh"} {
		t.Setenv("SHELL", environment)
		output, err := exec.Command(node, "-e", `process.stdout.write(JSON.stringify(require('node:os').userInfo().shell || process.env.SHELL || '/bin/sh'))`).CombinedOutput()
		if err != nil {
			t.Fatalf("Node shell oracle: %v: %s", err, output)
		}
		var want string
		if err := json.Unmarshal(output, &want); err != nil {
			t.Fatal(err)
		}
		got, err := DefaultShell()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("SHELL=%q: Go=%q Node=%q", environment, got, want)
		}
	}
}
