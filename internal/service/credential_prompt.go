package service

import (
	"context"
	"golang.org/x/crypto/ssh"
)

type CredentialField struct {
	Label  string `json:"label"`
	Secret bool   `json:"secret"`
}
type CredentialPrompt struct {
	Title   string
	Message string
	Fields  []CredentialField
}
type CredentialPrompter func(context.Context, CredentialPrompt) ([]string, error)
type NativeSSHAuthOptions struct {
	ReadIdentity    func(string) ([]byte, error)
	IdentityName    func(string) string
	Prompt          CredentialPrompter
	HostKeyCallback ssh.HostKeyCallback
}
