package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestVaultCreateUnlockAndPasswordChange(t *testing.T) {
	store := NewVaultStore()
	payload := VaultPayload{
		Snapshot:     []byte("opaque sqlite snapshot"),
		Passwords:    map[string]string{"host-1": "correct horse battery staple"},
		SourceDevice: "Work Mac",
		HostCount:    1,
	}
	encoded, key, err := store.Create("old password", payload)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if bytes.Contains(encoded, []byte("Work Mac")) || bytes.Contains(encoded, []byte("correct horse")) {
		t.Fatal("vault envelope exposes encrypted workspace data")
	}
	opened, openedKey, err := store.Unlock(encoded, "old password")
	if err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	if opened.Payload.Passwords["host-1"] != payload.Passwords["host-1"] || !bytes.Equal(openedKey, key) {
		t.Fatalf("Unlock() payload/key mismatch")
	}

	changed, err := store.ChangePassword(encoded, key, "new password")
	if err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if _, _, err := store.Unlock(changed, "old password"); !errors.Is(err, ErrInvalidVaultPassword) {
		t.Fatalf("Unlock(old password) error = %v, want ErrInvalidVaultPassword", err)
	}
	if _, _, err := store.Unlock(changed, "new password"); err != nil {
		t.Fatalf("Unlock(new password) error = %v", err)
	}
}

func TestVaultTamperingFailsAuthentication(t *testing.T) {
	store := NewVaultStore()
	encoded, _, err := store.Create("password", VaultPayload{Snapshot: []byte("snapshot")})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	var envelope vaultEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := decodeVaultField(envelope.Payload.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[len(ciphertext)/2] ^= 0x80
	envelope.Payload.Ciphertext = bytesToBase64(ciphertext)
	tampered, _ := json.Marshal(envelope)

	if _, _, err := store.Unlock(tampered, "password"); !errors.Is(err, ErrVaultCorrupted) {
		t.Fatalf("Unlock(tampered) error = %v, want ErrVaultCorrupted", err)
	}
}

func TestVaultRejectsUnsupportedVersion(t *testing.T) {
	store := NewVaultStore()
	encoded, _, err := store.Create("password", VaultPayload{})
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	_ = json.Unmarshal(encoded, &envelope)
	envelope["formatVersion"] = float64(99)
	encoded, _ = json.Marshal(envelope)
	if _, _, err := store.Unlock(encoded, "password"); !errors.Is(err, ErrVaultVersionUnsupported) {
		t.Fatalf("Unlock() error = %v, want ErrVaultVersionUnsupported", err)
	}
}

func bytesToBase64(value []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	out := make([]byte, 0, (len(value)*8+5)/6)
	var buffer uint32
	var bits uint
	for _, b := range value {
		buffer = buffer<<8 | uint32(b)
		bits += 8
		for bits >= 6 {
			bits -= 6
			out = append(out, alphabet[(buffer>>bits)&63])
		}
	}
	if bits > 0 {
		out = append(out, alphabet[(buffer<<(6-bits))&63])
	}
	return string(out)
}
