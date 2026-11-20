//go:build darwin && !mas

package app

import (
	"context"
	"errors"
	"log"
	"slices"
	"strings"
	"unicode"

	"ssh-connect/frontend/src/shared/i18n"
	"ssh-connect/internal/desktop/platform"
	"ssh-connect/internal/service"
)

// approveAgentRoute keeps the resolved route in the desktop process while
// asking the user to approve its saved destination.
func (a *App) approveAgentRoute(ctx context.Context, hostID string) (service.AgentRoute, error) {
	if !service.LocalTerminalAvailable() {
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
	target := route.Hosts()[0]
	var directory string
	if target.DirectoryID != "" {
		directories, err := svc.ListDirectories(ctx)
		if err != nil {
			return service.AgentRoute{}, err
		}
		directory = agentDirectoryPath(target.DirectoryID, directories)
	}
	for ctx.Err() == nil {
		revision := a.desktopLocaleRevision.Load()
		preferences, err := a.desktopPreferences()
		if err != nil {
			return service.AgentRoute{}, err
		}
		preference, _ := preferences["language"].(string)
		var systemLanguages []string
		if preference == "system" {
			systemLanguages = platform.PreferredLanguages()
		}
		messages := i18n.NativeMessages(preference, systemLanguages)
		body := agentApprovalBody(messages, target, directory, route.Endpoints()[0])
		localeChanged := func() bool { return a.desktopLocaleRevision.Load() != revision }
		if localeChanged() {
			continue
		}
		accepted, err := platform.ConfirmAgentConnection(ctx, messages["agent.openTitle"], body,
			messages["agent.rejectConnection"], messages["agent.allowConnection"], localeChanged)
		if err != nil {
			log.Printf("aTerm native AI connection dialog: %v", err)
			return service.AgentRoute{}, err
		}
		if localeChanged() {
			continue
		}
		if accepted && ctx.Err() == nil {
			return route, nil
		}
		break
	}
	return service.AgentRoute{}, errors.New("APPROVAL_DECLINED")
}

func agentDirectoryPath(id string, directories []service.Directory) string {
	byID := make(map[string]service.Directory, len(directories))
	for _, directory := range directories {
		byID[directory.ID] = directory
	}
	var parts []string
	for id != "" {
		directory, ok := byID[id]
		if !ok {
			break
		}
		delete(byID, id)
		parts = append(parts, directory.Name)
		id = directory.ParentID
	}
	slices.Reverse(parts)
	return strings.Join(parts, " / ")
}

func agentApprovalBody(messages map[string]string, target service.Host, directory, endpoint string) string {
	singleLine := func(value string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
				return ' '
			}
			return r
		}, value)
	}
	name := target.DisplayName()
	lines := []string{singleLine(name)}
	if target.Alias != "" && target.Alias != name {
		lines = append(lines, strings.ReplaceAll(messages["agent.openAlias"], "{{alias}}", singleLine(target.Alias)))
	}
	if directory != "" {
		lines = append(lines, strings.ReplaceAll(messages["agent.openDirectory"], "{{directory}}", singleLine(directory)))
	}
	lines = append(lines, strings.ReplaceAll(messages["agent.openTarget"], "{{target}}", singleLine(endpoint)))
	return strings.Join(lines, "\n")
}
