package service

import (
	"context"
	"errors"
	"strings"

	desktopkeys "ssh-connect/internal/desktop/keys"
)

// ConnectionRequiresUnlock includes credentials used by saved jump hosts.
func (s *AppService) ConnectionRequiresUnlock(ctx context.Context, id string) (bool, error) {
	host, err := s.storeHostByID(id)
	if err != nil {
		return false, err
	}
	for depth := 0; ; depth++ {
		if depth > maxProxyJumpDepth {
			return false, errors.New("SSH ProxyJump chain is too deep")
		}
		if host.PasswordAuth || strings.HasPrefix(host.IdentityFile, desktopkeys.Prefix) {
			status, err := s.GetEncryptedSyncStatus(ctx)
			return !status.Unlocked, err
		}
		if jumpID := strings.TrimSpace(host.JumpHostID); jumpID != "" {
			host, err = s.storeHostByID(jumpID)
		} else if alias := firstProxyJump(host.ProxyJump); alias != "" {
			host, err = s.hostByAlias(alias)
		} else {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
