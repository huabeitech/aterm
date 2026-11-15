package app

import (
	"context"
	"fmt"
	"ssh-connect/internal/desktop/prompts"
	"ssh-connect/internal/service"
)

func (a *App) promptBroker() *prompts.Broker {
	a.desktopMu.Lock()
	defer a.desktopMu.Unlock()
	if a.desktopPrompts == nil {
		a.desktopPrompts = prompts.New(func(event any) { a.emitEvent("terminal:event", event) })
	}
	return a.desktopPrompts
}
func (a *App) credentialPrompter(sessionID string) service.CredentialPrompter {
	return func(ctx context.Context, value service.CredentialPrompt) ([]string, error) {
		fields := make([]prompts.Field, len(value.Fields))
		for i, field := range value.Fields {
			fields[i] = prompts.Field{Label: field.Label, Secret: field.Secret}
		}
		return a.promptBroker().Ask(ctx, prompts.Prompt{SessionID: sessionID, Kind: "credentials", Title: value.Title, Message: value.Message, Fields: fields})
	}
}
func (a *App) hostKeyConfirmer(sessionID string) service.HostKeyConfirmer {
	return func(ctx context.Context, key service.HostKeyInfo) (bool, error) {
		kind, title := "host-key", "Verify SSH Host Key"
		message := fmt.Sprintf("Host: %s\nPort: %s\nKey type: %s\nSHA-256 fingerprint: %s\n\nVerify this fingerprint with the server administrator before trusting this host.", key.Host, key.Port, key.KeyType, key.Fingerprint)
		if key.PreviousFingerprint != "" {
			kind, title = "host-key-changed", "SSH Host Key Changed"
			message = fmt.Sprintf("Host: %s\nPort: %s\nPrevious fingerprint: %s\nReceived fingerprint: %s\n\nThe server identity has changed. Verify the new fingerprint with the server administrator before replacing the trusted key.", key.Host, key.Port, key.PreviousFingerprint, key.Fingerprint)
		}
		_, err := a.promptBroker().Ask(ctx, prompts.Prompt{SessionID: sessionID, Kind: kind, Title: title, Message: message})
		return err == nil, err
	}
}
