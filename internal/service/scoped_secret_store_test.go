package service

import "testing"

func TestExplicitProfileCredentialsDoNotReachDefaultAccounts(t *testing.T) {
	base := newMemoryPasswordStore()
	base.Set(VaultUnlockAccount, "production")
	profile := ScopedSecretStore(base, "profile-a")
	other := ScopedSecretStore(base, "profile-b")
	profile.Set(VaultUnlockAccount, "fixture")
	profile.Set("aterm-managed-key:id", "private fixture")
	if value, _ := base.Get(VaultUnlockAccount); value != "production" {
		t.Fatal("profile replaced default key")
	}
	if _, err := other.Get(VaultUnlockAccount); err == nil {
		t.Fatal("profiles share a key")
	}
	profile.Delete(VaultUnlockAccount)
	if value, _ := base.Get(VaultUnlockAccount); value != "production" {
		t.Fatal("profile deleted default key")
	}
	if value, _ := profile.Get("aterm-managed-key:id"); value != "private fixture" {
		t.Fatal("managed key not scoped")
	}
}
