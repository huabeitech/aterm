package app

import (
	"context"
	"errors"
	"runtime"
	"strings"

	"ssh-connect/internal/service"
)

// approveAgentRoute is called when local agent access is enabled. The returned
// route is kept by the desktop process while the user unlocks their vault.
func (a *App) approveAgentRoute(ctx context.Context, hostID string) (service.AgentRoute, error) {
	if runtime.GOOS != "darwin" || !service.LocalTerminalAvailable() {
		return service.AgentRoute{}, errors.New("AGENT_UNAVAILABLE")
	}
	svc, err := a.service()
	if err != nil {
		return service.AgentRoute{}, err
	}
	route, err := svc.PrepareAgentRoute(ctx, hostID)
	if err != nil {
		return service.AgentRoute{}, err
	}
	endpoints := route.Endpoints()
	accepted := a.confirmAction(ctx, windowConfirmation{
		TitleKey: "agent.openTitle", MessageKey: "agent.openMessage",
		ConfirmKey: "nativePlugin.allow", CancelKey: "nativePlugin.cancel",
		Parameters: map[string]string{"target": endpoints[0], "route": strings.Join(endpoints, "\n")},
	})
	if !accepted {
		return service.AgentRoute{}, errors.New("APPROVAL_DECLINED")
	}
	return route, nil
}
