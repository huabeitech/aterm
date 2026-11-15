// Command parity-fixture creates a disposable loopback-only SSH/SFTP server and
// an isolated Wails profile for native migration acceptance. It never reads the
// user's SSH configuration, keys, vault or production library.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"ssh-connect/internal/db"
	"ssh-connect/internal/desktop/terminalmeta"
	"ssh-connect/internal/domain"
)

func main() {
	profile := flag.String("profile", "", "new disposable profile under the system temporary directory")
	remoteCwd := flag.String("cwd", "", "fixture remote cwd reported to metadata probes")
	trustMode := flag.String("trust", "known", "fixture host trust: known, unknown or changed")
	authMode := flag.String("auth", "publickey", "fixture authentication: publickey, password or interactive")
	flag.Parse()
	if !filepath.IsAbs(*profile) {
		log.Fatal("provide an absolute temporary profile path")
	}
	relative, err := filepath.Rel(os.TempDir(), *profile)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		log.Fatal("fixture profiles must be inside the system temporary directory")
	}
	database := filepath.Join(*profile, "app.db")
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		log.Fatal("refusing to replace an existing profile database")
	}
	sshDirectory := filepath.Join(*profile, "ssh")
	if err := os.MkdirAll(sshDirectory, 0o700); err != nil {
		log.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "disposable parity fixture")
	if err != nil {
		log.Fatal(err)
	}
	identity := filepath.Join(sshDirectory, "id_ed25519")
	if err := os.WriteFile(identity, pem.EncodeToMemory(block), 0o600); err != nil {
		log.Fatal(err)
	}
	expected, _ := ssh.NewPublicKey(public)
	_, serverPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(serverPrivate)
	if err != nil {
		log.Fatal(err)
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if !bytes.Equal(key.Marshal(), expected.Marshal()) {
			return nil, fmt.Errorf("unexpected fixture key")
		}
		return &ssh.Permissions{}, nil
	}}
	switch *authMode {
	case "publickey":
	case "password":
		config.PublicKeyCallback = nil
		config.PasswordCallback = func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != "fixture-password" {
				return nil, fmt.Errorf("incorrect fixture password")
			}
			return &ssh.Permissions{}, nil
		}
	case "interactive":
		config.PublicKeyCallback = nil
		config.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			answers, err := challenge("fixture", "Disposable verification challenge", []string{"Login", "Verification code"}, []bool{true, false})
			if err != nil {
				return nil, err
			}
			if len(answers) != 2 || answers[0] != "fixture" || answers[1] != "123456" {
				return nil, fmt.Errorf("incorrect fixture challenge")
			}
			return &ssh.Permissions{}, nil
		}
	default:
		log.Fatal("unknown fixture authentication mode")
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	trust := knownhosts.Line([]string{net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}, signer.PublicKey()) + "\n"
	if err := os.WriteFile(filepath.Join(sshDirectory, "known_hosts"), []byte(trust), 0o600); err != nil {
		log.Fatal(err)
	}
	switch *trustMode {
	case "known", "changed":
		fingerprint := ssh.FingerprintSHA256(signer.PublicKey())
		if *trustMode == "changed" {
			fingerprint = ssh.FingerprintSHA256(expected)
		}
		data, err := json.Marshal(map[string]any{"version": 1, "keys": map[string]string{"127.0.0.1:" + strconv.Itoa(port): fingerprint}})
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*profile, "ssh-host-keys.json"), data, 0600); err != nil {
			log.Fatal(err)
		}
	case "unknown":
	default:
		log.Fatal("invalid fixture host trust mode")
	}
	store, err := db.Open(database)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := store.CreateHost(domain.HostInput{Alias: "parity-sftp", HostName: "127.0.0.1", User: "fixture", Port: port, IdentityFile: identity}); err != nil {
		log.Fatal(err)
	}
	store.Close()
	handlers := sftp.InMemHandler()
	seed(handlers)
	fmt.Printf("Fixture ready: profile=%s address=%s\n", *profile, listener.Addr())
	for {
		connection, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go serve(connection, config, handlers, *remoteCwd)
	}
}
func seed(handlers sftp.Handlers) {
	left, right := net.Pipe()
	server := sftp.NewRequestServer(right, handlers)
	go func() { server.Serve(); server.Close() }()
	client, err := sftp.NewClientPipe(left, left)
	if err != nil {
		log.Fatal(err)
	}
	if err := client.Mkdir("/folder"); err != nil {
		log.Fatal(err)
	}
	for name, content := range map[string]string{"/config.txt": "# Disposable aTerm SFTP fixture\nhello=world\n中文=你好\n", "/folder/nested.txt": "nested fixture\n"} {
		file, err := client.Create(name)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := io.WriteString(file, content); err != nil {
			log.Fatal(err)
		}
		file.Close()
	}
	client.Close()
	left.Close()
	right.Close()
}
func serve(connection net.Conn, config *ssh.ServerConfig, handlers sftp.Handlers, remoteCwd string) {
	server, channels, requests, err := ssh.NewServerConn(connection, config)
	if err != nil {
		connection.Close()
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for request := range channels {
		if request.ChannelType() != "session" {
			request.Reject(ssh.UnknownChannelType, "fixture accepts session channels only")
			continue
		}
		channel, requests, err := request.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for request := range requests {
				switch request.Type {
				case "subsystem":
					var value struct{ Name string }
					if err := ssh.Unmarshal(request.Payload, &value); err != nil || value.Name != "sftp" {
						request.Reply(false, nil)
						continue
					}
					request.Reply(true, nil)
					server := sftp.NewRequestServer(channel, handlers)
					server.Serve()
					server.Close()
					return
				case "exec":
					var value struct{ Command string }
					if err := ssh.Unmarshal(request.Payload, &value); err != nil || remoteCwd == "" || value.Command != terminalmeta.RemoteDirectoryCommand {
						request.Reply(false, nil)
						continue
					}
					request.Reply(true, nil)
					io.WriteString(channel, remoteCwd+"\n")
					channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
					return
				case "pty-req", "env", "window-change":
					request.Reply(true, nil)
				case "shell":
					request.Reply(true, nil)
					io.WriteString(channel, "Disposable parity fixture shell. Use Open SFTP to inspect files.\r\n")
					io.Copy(io.Discard, channel)
					return
				default:
					request.Reply(false, nil)
				}
			}
		}()
	}
}
