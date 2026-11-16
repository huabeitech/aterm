package app

import (
	"context"
	"encoding/json"
	"fmt"
	"ssh-connect/internal/domain"
)

func (a *App) desktopTunnels(ctx context.Context, raw []json.RawMessage) (any, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	op, args, err := operation(raw)
	if err != nil {
		return nil, err
	}
	if op == "list" {
		return svc.ListTunnels(ctx)
	}
	id, err := argument[string](args, 0)
	if err != nil {
		return nil, err
	}
	switch op {
	case "save":
		input, err := argument[domain.TunnelInput](args, 1)
		if err != nil {
			return nil, err
		}
		return svc.SaveTunnel(ctx, id, input)
	case "start":
		return nil, svc.StartTunnel(ctx, id, a.hostKeyConfirmer("tunnel:"+id), a.credentialPrompter("tunnel:"+id))
	case "stop":
		return nil, svc.StopTunnel(ctx, id)
	case "delete":
		return nil, svc.DeleteTunnel(ctx, id)
	default:
		return nil, fmt.Errorf("unknown tunnel operation %q", op)
	}
}
