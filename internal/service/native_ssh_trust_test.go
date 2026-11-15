package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"ssh-connect/internal/ssh/hostkeys"
)

func trustTestSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}
func TestNativeSSHHostKeyRotationRequiresExplicitReplacement(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	first, second := trustTestSigner(t), trustTestSigner(t)
	var mu sync.Mutex
	current := first
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			signer := current
			mu.Unlock()
			go func() {
				config := &ssh.ServerConfig{NoClientAuth: true}
				config.AddHostKey(signer)
				server, channels, requests, err := ssh.NewServerConn(connection, config)
				if err != nil {
					connection.Close()
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					channel.Reject(ssh.Prohibited, "fixture")
				}
			}()
		}
	}()
	hostName, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	host := Host{Alias: "rotation", HostName: hostName, Port: port, User: "fixture", ConnectTimeout: 1}
	root := t.TempDir()
	path := filepath.Join(root, "ssh-host-keys.json")
	app := &AppService{hostKeyStore: hostkeys.New(path)}
	sshDirectory := filepath.Join(root, "ssh")
	connect := func(confirm HostKeyConfirmer) error {
		client, err := app.dialNativeSSH(context.Background(), host, sshDirectory, confirm, 0, NativeSSHAuthOptions{Prompt: func(context.Context, CredentialPrompt) ([]string, error) { return []string{"fixture"}, nil }})
		if client != nil {
			client.Close()
		}
		return err
	}
	asked := 0
	if err = connect(func(_ context.Context, key HostKeyInfo) (bool, error) {
		asked++
		if key.PreviousFingerprint != "" || key.Fingerprint != ssh.FingerprintSHA256(first.PublicKey()) {
			t.Error("incorrect initial fingerprint", key)
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	neverAsk := func(context.Context, HostKeyInfo) (bool, error) {
		t.Error("known key prompted again")
		return false, nil
	}
	if err = connect(neverAsk); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	mu.Lock()
	current = second
	mu.Unlock()
	if err = connect(func(_ context.Context, key HostKeyInfo) (bool, error) {
		asked++
		if key.PreviousFingerprint != ssh.FingerprintSHA256(first.PublicKey()) || key.Fingerprint != ssh.FingerprintSHA256(second.PublicKey()) {
			t.Error("missing changed fingerprints", key)
		}
		return false, nil
	}); err == nil {
		t.Fatal("changed key accepted without approval")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("cancel changed the trusted key")
	}
	if err = connect(func(_ context.Context, key HostKeyInfo) (bool, error) {
		asked++
		if key.PreviousFingerprint == "" {
			t.Error("replacement lost prior fingerprint")
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	app.hostKeyStore = hostkeys.New(path)
	if err = connect(neverAsk); err != nil {
		t.Fatal(err)
	}
	if asked != 3 {
		t.Fatalf("unexpected prompt count %d", asked)
	}
	if _, err = os.Stat(filepath.Join(sshDirectory, "known_hosts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("application trust modified OpenSSH known_hosts")
	}
}
func TestNativeSSHStaleHostKeyApprovalCannotOverwriteTrust(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-host-keys.json")
	store := hostkeys.New(path)
	first, second, third := trustTestSigner(t), trustTestSigner(t), trustTestSigner(t)
	old := ssh.FingerprintSHA256(first.PublicKey())
	newer := ssh.FingerprintSHA256(third.PublicKey())
	if err := store.Commit(context.Background(), "host", "22", "", old); err != nil {
		t.Fatal(err)
	}
	app := &AppService{hostKeyStore: store}
	callback := app.trustedHostKeyCallback(context.Background(), func(ctx context.Context, key HostKeyInfo) (bool, error) {
		if err := store.Commit(ctx, key.Host, key.Port, old, newer); err != nil {
			t.Fatal(err)
		}
		return true, nil
	})
	if err := callback("host:22", &net.TCPAddr{}, second.PublicKey()); !errors.Is(err, hostkeys.ErrChanged) {
		t.Fatalf("stale approval: %v", err)
	}
	if got, err := store.Lookup("host", "22"); err != nil || got != newer {
		t.Fatal("new trust was overwritten", got, err)
	}
}

func TestElectronKnownHostsInterop(t *testing.T) {
	reference := os.Getenv("ATERM_ELECTRON_REFERENCE_DIR")
	if reference == "" {
		t.Skip("set ATERM_ELECTRON_REFERENCE_DIR for Electron trust-file interop")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "ssh-host-keys.json")
	first, second := trustTestSigner(t), trustTestSigner(t)
	store := hostkeys.New(path)
	if err := store.Commit(context.Background(), "Example.test", "2222", "", ssh.FingerprintSHA256(first.PublicKey())); err != nil {
		t.Fatal(err)
	}
	script := `
const fs=require('node:fs'),path=require('node:path');
const [reference,directory,first,second]=process.argv.slice(1);
const ts=require(path.join(reference,'node_modules/typescript'));
const source=fs.readFileSync(path.join(reference,'src/main/terminal/knownHosts.ts'),'utf8');
const compiled=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.CommonJS}}).outputText;
const value={exports:{}};
Function('module','exports','require',compiled)(value,value.exports,name=>name==='../../shared/errors'?{AppError:class extends Error{},appError:message=>Error(message)}:require(name));
const known=new value.exports.KnownHosts(directory);
if(known.check('EXAMPLE.TEST',2222,Buffer.from(first,'base64'))!=='known')throw Error('Electron did not accept Wails trust file');
known.replace('example.test',2222,Buffer.from(second,'base64'));
if(known.check('Example.test',2222,Buffer.from(second,'base64'))!=='known')throw Error('Electron replacement failed');
`
	command := exec.Command("node", "-e", script, reference, directory, base64.StdEncoding.EncodeToString(first.PublicKey().Marshal()), base64.StdEncoding.EncodeToString(second.PublicKey().Marshal()))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Electron trust-file interoperability: %v\n%s", err, output)
	}
	if got, err := store.Lookup("EXAMPLE.TEST", "2222"); err != nil || got != ssh.FingerprintSHA256(second.PublicKey()) {
		t.Fatal("Wails did not accept Electron replacement", got, err)
	}
}
