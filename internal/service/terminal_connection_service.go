package service

import (
	"context"
	"strings"
	"time"
)

func (s *ConnectionService) PrepareTerminalConnection(ctx context.Context, alias string) (Host, error) {
	if err := ctx.Err(); err != nil {
		return Host{}, err
	}
	if err := s.requireConnectionFeatures(ctx); err != nil {
		return Host{}, err
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return Host{}, ErrAliasRequired
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		return Host{}, err
	}
	for _, host := range hosts {
		if host.Alias != alias {
			continue
		}
		updated, err := s.store.MarkHostUsed(host.ID, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return Host{}, err
		}
		return updated, nil
	}
	return Host{}, ErrHostNotFound
}
