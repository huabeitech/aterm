package service

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestElectronKeyboardInteractiveInteroperability(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for actual interactive prompt oracle")
	}
	type vector struct {
		Questions   []string `json:"questions"`
		Echo        []bool   `json:"echo"`
		Instruction string   `json:"instruction"`
	}
	vectors := []vector{}
	for _, question := range []string{"Password:", "fixture's password:", "fixture’s password:", "user@host's password:   ", "One-time password:", "One time password:", "Verification code:", "Password for root:", "PASSWORD:\u00a0", "password:\ufeff", "K password:", "ſ password:"} {
		vectors = append(vectors, vector{Questions: []string{question}, Echo: []bool{false}})
	}
	vectors = append(vectors, vector{Questions: []string{}, Echo: []bool{}}, vector{Questions: []string{"Login", "Verification code"}, Echo: []bool{true, false}, Instruction: "Enter your login and token"}, vector{Questions: []string{"Password:"}, Echo: []bool{true}})
	data, err := json.Marshal(vectors)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "vectors.json")
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("testdata/electron-interactive-oracle.mts")
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(filepath.Join(reference, "node_modules/.bin/tsx"), script, reference, input).CombinedOutput()
	if err != nil {
		t.Fatalf("Electron interactive oracle: %v: %s", err, output)
	}
	type result struct {
		Answers []string           `json:"answers"`
		Prompts []CredentialPrompt `json:"prompts"`
	}
	var expected []result
	if err := json.Unmarshal(output, &expected); err != nil {
		t.Fatalf("invalid oracle output: %v: %s", err, output)
	}
	if len(expected) != len(vectors) {
		t.Fatal("invalid oracle count")
	}
	for index, vector := range vectors {
		got := result{Prompts: []CredentialPrompt{}}
		answers := make(chan []string, 1)
		options := NativeSSHAuthOptions{Prompt: func(_ context.Context, prompt CredentialPrompt) ([]string, error) {
			got.Prompts = append(got.Prompts, prompt)
			values := make([]string, len(vector.Questions))
			for index := range values {
				values[index] = "123456"
			}
			return values, nil
		}}
		server := &ssh.ServerConfig{KeyboardInteractiveCallback: func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			response, err := challenge("Fixture", vector.Instruction, vector.Questions, vector.Echo)
			if response == nil {
				response = []string{}
			}
			answers <- response
			return nil, err
		}}
		if err := dialNativeAuthTest(t, Host{Alias: "Fixture", HostName: "fixture.invalid", PasswordAuth: true}, t.TempDir(), "saved-password", options, server); err != nil {
			t.Fatal(err)
		}
		got.Answers = <-answers
		if !reflect.DeepEqual(got, expected[index]) {
			t.Errorf("vector %d: Go=%+v Electron=%+v", index, got, expected[index])
		}
	}
	t.Logf("compared %d real keyboard-interactive exchanges to the Electron handler", len(vectors))
}
