package terminal

import (
	"strings"
	"testing"

	"ssh-connect/internal/service"
)

func TestParseTerminalHostKeyAnswerAcceptsYesPrefixesCaseInsensitively(t *testing.T) {
	fingerprint := "SHA256:AbCdEf123+/="
	for _, answer := range []string{"y", "Y", "ye", "YE", "yes", "YeS", " yes "} {
		if got := parseTerminalHostKeyAnswer(answer, fingerprint); got != terminalHostKeyTrust {
			t.Errorf("answer %q = %v, want trust", answer, got)
		}
	}
}

func TestParseTerminalHostKeyAnswerAcceptsNoPrefixesCaseInsensitively(t *testing.T) {
	fingerprint := "SHA256:AbCdEf123+/="
	for _, answer := range []string{"n", "N", "no", "NO", " No "} {
		if got := parseTerminalHostKeyAnswer(answer, fingerprint); got != terminalHostKeyReject {
			t.Errorf("answer %q = %v, want reject", answer, got)
		}
	}
}

func TestParseTerminalHostKeyAnswerRequiresCompleteFingerprint(t *testing.T) {
	fingerprint := "SHA256:AbCdEf123+/="
	for _, answer := range []string{fingerprint, "sha256:AbCdEf123+/=", " SHA256:AbCdEf123+/= "} {
		if got := parseTerminalHostKeyAnswer(answer, fingerprint); got != terminalHostKeyTrust {
			t.Errorf("answer %q = %v, want trust", answer, got)
		}
	}
	for _, answer := range []string{
		"SHA256:AbCdEf123+/",
		"SHA256:abcdef123+/=",
		"MD5:AbCdEf123+/=",
		"yes please",
		"",
	} {
		if got := parseTerminalHostKeyAnswer(answer, fingerprint); got != terminalHostKeyPending {
			t.Errorf("answer %q = %v, want pending", answer, got)
		}
	}
}

func TestTerminalHostKeyPromptUsesOpenSSHStyleText(t *testing.T) {
	message := terminalHostKeyPromptMessage(service.HostKeyInfo{
		Host:        "example.test",
		Port:        "2222",
		KeyType:     "ssh-ed25519",
		Fingerprint: "SHA256:AbCdEf123+/=",
	})
	for _, expected := range []string{
		"The authenticity of host '[example.test]:2222' can't be established.",
		"ED25519 key fingerprint is SHA256:AbCdEf123+/=.",
		"Are you sure you want to continue connecting (yes/no/[fingerprint])?",
	} {
		if !strings.Contains(message, expected) {
			t.Errorf("prompt %q does not contain %q", message, expected)
		}
	}
}

func TestProcessTerminalHostKeyInputHandlesEditingAndDiscardsTrailingPaste(t *testing.T) {
	prompt := &terminalHostKeyPrompt{
		fingerprint: "SHA256:AbCdEf123+/=",
		response:    make(chan terminalHostKeyDecision, 1),
	}
	output := processTerminalHostKeyInput(prompt, []byte("yex\x7fs\rwhoami\r"))
	if strings.Contains(output, "whoami") {
		t.Fatalf("output retained trailing pasted command: %q", output)
	}
	if decision := <-prompt.response; decision != terminalHostKeyTrust {
		t.Fatalf("decision = %v, want trust", decision)
	}
}

func TestProcessTerminalHostKeyInputRepromptsWithoutConsumingAnotherPastedLine(t *testing.T) {
	prompt := &terminalHostKeyPrompt{
		fingerprint: "SHA256:AbCdEf123+/=",
		response:    make(chan terminalHostKeyDecision, 1),
	}
	output := processTerminalHostKeyInput(prompt, []byte("maybe\rYES\r"))
	if !strings.Contains(output, "Please type yes (y), no (n), or the full fingerprint:") {
		t.Fatalf("output = %q, want retry prompt", output)
	}
	if strings.Contains(output, "YES") {
		t.Fatalf("output retained second pasted line: %q", output)
	}
	select {
	case decision := <-prompt.response:
		t.Fatalf("unexpected decision %v", decision)
	default:
	}
}

func TestProcessTerminalHostKeyInputTreatsControlCAsRejection(t *testing.T) {
	prompt := &terminalHostKeyPrompt{
		fingerprint: "SHA256:AbCdEf123+/=",
		response:    make(chan terminalHostKeyDecision, 1),
	}
	if output := processTerminalHostKeyInput(prompt, []byte{0x03}); output != "^C\r\n" {
		t.Fatalf("output = %q, want control-C echo", output)
	}
	if decision := <-prompt.response; decision != terminalHostKeyReject {
		t.Fatalf("decision = %v, want reject", decision)
	}
}
