package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestExpandSSHPathUsesAuthorizedDirectory(t *testing.T) {
	sshDir := filepath.Join(t.TempDir(), ".ssh")
	if got := expandSSHPath("~/.ssh/id_ed25519", sshDir); got != filepath.Join(sshDir, "id_ed25519") {
		t.Fatalf("expandSSHPath() = %q", got)
	}
}

func TestFirstProxyJumpExtractsStoredAlias(t *testing.T) {
	if got := firstProxyJump("deploy@bastion:2222,backup"); got != "bastion" {
		t.Fatalf("firstProxyJump() = %q, want bastion", got)
	}
}

func TestNativeSSHAuthenticatesWithDefaultPrivateKey(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	sshDir := t.TempDir()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(sshDir, "id_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0o600); err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := dialNativeAuthTest(t, Host{}, sshDir, "", NativeSSHAuthOptions{}, &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if ssh.FingerprintSHA256(key) != ssh.FingerprintSHA256(signer.PublicKey()) {
			return nil, errors.New("wrong identity")
		}
		return nil, nil
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeSSHAuthenticatesWithSavedPassword(t *testing.T) {
	if err := dialNativeAuthTest(t, Host{PasswordAuth: true}, t.TempDir(), "correct horse battery staple", NativeSSHAuthOptions{}, &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "correct horse battery staple" {
			return nil, errors.New("wrong password")
		}
		return nil, nil
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordForHostUsesActivePasswordStore(t *testing.T) {
	delegate := newMemoryPasswordStore()
	hostPasswords := newSwitchablePasswordStore(delegate)
	hostPasswords.Activate(newVaultPasswordStore(map[string]string{"host-1": "vault-password"}, nil))
	app := &AppService{passwordStore: delegate, hostPasswordStore: hostPasswords}

	password, err := app.passwordForHost(Host{ID: "host-1", PasswordAuth: true})
	if err != nil {
		t.Fatalf("passwordForHost() error = %v", err)
	}
	if password != "vault-password" {
		t.Fatalf("passwordForHost() = %q, want vault-password", password)
	}
}

func TestNativeSSHEmptyAgentFallsBackToDefaultPrivateKey(t *testing.T) {
	sshDir := t.TempDir()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0o600); err != nil {
		t.Fatal(err)
	}
	agentPath := filepath.Join(os.TempDir(), "ssh-term-agent-"+time.Now().UTC().Format("20060102150405.000000000")+".sock")
	listener, err := net.Listen("unix", agentPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(agentPath) })
	defer listener.Close()
	t.Setenv("SSH_AUTH_SOCK", listener.Addr().String())
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := dialNativeAuthTest(t, Host{}, sshDir, "", NativeSSHAuthOptions{}, &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if ssh.FingerprintSHA256(key) != ssh.FingerprintSHA256(signer.PublicKey()) {
			return nil, errors.New("wrong identity")
		}
		return nil, nil
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeSSHRSAAuthenticationUsesSHA2(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	sshDir := t.TempDir()
	key := testRSAPrivateKeyPEM(t)
	if err := os.WriteFile(filepath.Join(sshDir, "id_rsa"), key, 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := ssh.ParsePrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	compatible, err := nativeCompatibleSigner(parsed)
	if err != nil {
		t.Fatal(err)
	}
	multi, ok := compatible.(ssh.MultiAlgorithmSigner)
	if !ok {
		t.Fatal("RSA signer does not expose explicit algorithms")
	}
	algorithms := multi.Algorithms()
	if len(algorithms) != 2 || algorithms[0] != ssh.KeyAlgoRSASHA512 || algorithms[1] != ssh.KeyAlgoRSASHA256 {
		t.Fatalf("RSA algorithms = %v", algorithms)
	}
	if err := dialNativeAuthTest(t, Host{}, sshDir, "", NativeSSHAuthOptions{}, &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if ssh.FingerprintSHA256(key) != ssh.FingerprintSHA256(parsed.PublicKey()) {
				return nil, errors.New("wrong RSA identity")
			}
			return nil, nil
		},
		VerifiedPublicKeyCallback: func(_ ssh.ConnMetadata, _ ssh.PublicKey, permissions *ssh.Permissions, algorithm string) (*ssh.Permissions, error) {
			if algorithm != ssh.KeyAlgoRSASHA512 && algorithm != ssh.KeyAlgoRSASHA256 {
				return nil, errors.New("legacy RSA signature used")
			}
			return permissions, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeKnownHostsTrustsFirstKeyAndRejectsChanges(t *testing.T) {
	sshDir := t.TempDir()
	confirmations := 0
	callback, err := nativeKnownHostsCallback(context.Background(), sshDir, func(_ context.Context, key HostKeyInfo) (bool, error) {
		confirmations++
		if key.Host != "example.test" || key.Port != "22" || key.KeyType == "" || key.Fingerprint == "" {
			t.Fatalf("unexpected host key confirmation: %+v", key)
		}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	first := testPublicKey(t)
	address := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 22}
	if err := callback("example.test:22", address, first); err != nil {
		t.Fatalf("first host key: %v", err)
	}
	callback, err = nativeKnownHostsCallback(context.Background(), sshDir, func(context.Context, HostKeyInfo) (bool, error) {
		t.Fatal("known host key requested confirmation again")
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("example.test:22", address, first); err != nil {
		t.Fatalf("known host key: %v", err)
	}
	if err := callback("example.test:22", address, testPublicKey(t)); err == nil {
		t.Fatal("changed host key was accepted")
	}
	if confirmations != 1 {
		t.Fatalf("host key confirmation count = %d, want 1", confirmations)
	}
}

func TestNativeKnownHostsCreatesMissingSSHDirectoryOnConnect(t *testing.T) {
	sshDir := filepath.Join(t.TempDir(), ".ssh")
	if _, err := nativeKnownHostsCallback(context.Background(), sshDir, nil); err != nil {
		t.Fatalf("nativeKnownHostsCallback() error = %v", err)
	}
	directoryInfo, err := os.Stat(sshDir)
	if err != nil {
		t.Fatalf("stat SSH directory: %v", err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("SSH directory permissions = %o, want 700", directoryInfo.Mode().Perm())
	}
	knownHostsInfo, err := os.Stat(filepath.Join(sshDir, "known_hosts"))
	if err != nil {
		t.Fatalf("stat known_hosts: %v", err)
	}
	if knownHostsInfo.Mode().Perm() != 0o600 {
		t.Fatalf("known_hosts permissions = %o, want 600", knownHostsInfo.Mode().Perm())
	}
}

func TestNativeKnownHostsDoesNotWriteRejectedFirstKey(t *testing.T) {
	sshDir := t.TempDir()
	callback, err := nativeKnownHostsCallback(context.Background(), sshDir, func(context.Context, HostKeyInfo) (bool, error) {
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("rejected.example:2222", &net.TCPAddr{IP: net.ParseIP("192.0.2.20"), Port: 2222}, testPublicKey(t)); err == nil {
		t.Fatal("rejected host key was accepted")
	}
	data, err := os.ReadFile(filepath.Join(sshDir, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("known_hosts = %q, want unchanged", data)
	}
}

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testRSAPrivateKeyPEM(t *testing.T) []byte {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
}
