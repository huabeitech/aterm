package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (s *HostImportService) PreviewHostSnippetImport(ctx context.Context, content string) (HostSnippetPreview, error) {
	if err := ctx.Err(); err != nil {
		return HostSnippetPreview{}, err
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		return HostSnippetPreview{}, err
	}
	return buildHostSnippetPreview(content, hosts), nil
}

func (s *HostImportService) ImportHostSnippet(ctx context.Context, input HostSnippetImportInput) (HostSnippetImportResult, error) {
	if err := ctx.Err(); err != nil {
		return HostSnippetImportResult{}, err
	}
	hosts, err := s.store.ListHosts()
	if err != nil {
		return HostSnippetImportResult{}, err
	}
	preview := buildHostSnippetPreview(input.Content, hosts)
	selected := make(map[string]bool, len(input.RowIDs))
	for _, rowID := range input.RowIDs {
		selected[rowID] = true
	}
	result := HostSnippetImportResult{}
	for _, row := range preview.Rows {
		if !selected[row.RowID] {
			continue
		}
		if !row.Selectable {
			result.Skipped++
			continue
		}
		created, err := s.store.CreateHost(row.Input)
		if err != nil {
			return HostSnippetImportResult{}, err
		}
		result.Created = append(result.Created, created)
	}
	return result, nil
}

type parsedHostSnippetRow struct {
	input  HostInput
	status string
}

func buildHostSnippetPreview(content string, existingHosts []Host) HostSnippetPreview {
	rows := parseHostSnippet(content)
	existingAliases := make(map[string]bool, len(existingHosts))
	for _, host := range existingHosts {
		existingAliases[host.Alias] = true
	}
	seenAliases := make(map[string]bool)
	previewRows := make([]HostSnippetPreviewRow, 0, len(rows))
	for index, row := range rows {
		status := row.status
		input := NormalizeHostInput(row.input)
		if status == "" {
			switch {
			case existingAliases[input.Alias]:
				status = HostSnippetStatusAlreadyExists
			case seenAliases[input.Alias]:
				status = HostSnippetStatusDuplicateInSnippet
			case input.HostName == "":
				status = HostSnippetStatusMissingHostName
			default:
				status = HostSnippetStatusReady
			}
		}
		if status == HostSnippetStatusReady {
			seenAliases[input.Alias] = true
		}
		previewRows = append(previewRows, HostSnippetPreviewRow{
			RowID:        fmt.Sprintf("host-%d", index+1),
			Alias:        input.Alias,
			HostName:     input.HostName,
			User:         input.User,
			Port:         input.Port,
			IdentityFile: input.IdentityFile,
			ProxyJump:    input.ProxyJump,
			Status:       status,
			Selectable:   status == HostSnippetStatusReady,
			Input:        input,
		})
	}
	return HostSnippetPreview{Content: content, Rows: previewRows}
}

func parseHostSnippet(content string) []parsedHostSnippetRow {
	var rows []parsedHostSnippetRow
	var current *parsedHostSnippetRow
	flush := func() {
		if current == nil {
			return
		}
		rows = append(rows, *current)
		current = nil
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := splitSSHConfigLine(trimmed)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "host":
			flush()
			current = &parsedHostSnippetRow{
				input: HostInput{
					Alias: strings.TrimSpace(value),
				},
			}
			hostFields := strings.Fields(value)
			switch {
			case len(hostFields) == 0:
				current.status = ErrAliasRequired.Error()
			case len(hostFields) > 1:
				current.status = HostSnippetStatusMultipleAliasesUnsupported
			case hostFields[0] == "*":
				current.status = HostSnippetStatusWildcardUnsupported
			default:
				current.input.Alias = hostFields[0]
			}
		case "match":
			flush()
		default:
			if current == nil {
				continue
			}
			applyHostSnippetOption(current, key, value)
		}
	}
	flush()
	return rows
}

func splitSSHConfigLine(line string) (string, string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", "", false
	}
	key := fields[0]
	value := strings.TrimSpace(strings.TrimPrefix(line, key))
	return key, value, true
}

func applyHostSnippetOption(row *parsedHostSnippetRow, key string, value string) {
	if row.status != "" {
		return
	}
	value = strings.TrimSpace(value)
	switch strings.ToLower(key) {
	case "hostname":
		row.input.HostName = value
	case "user":
		row.input.User = value
	case "port":
		port, err := parseHostSnippetInt(value)
		if err != nil {
			row.status = HostSnippetStatusInvalidPort
			return
		}
		row.input.Port = port
	case "identityfile":
		row.input.IdentityFile = value
	case "proxyjump":
		row.input.ProxyJump = value
	case "forwardagent":
		value, ok := parseHostSnippetBool(value)
		if !ok {
			row.status = HostSnippetStatusInvalidOptionValue
			return
		}
		row.input.ForwardAgent = value
	case "compression":
		value, ok := parseHostSnippetBool(value)
		if !ok {
			row.status = HostSnippetStatusInvalidOptionValue
			return
		}
		row.input.Compression = value
	case "serveraliveinterval":
		value, err := parseHostSnippetInt(value)
		if err != nil {
			row.status = HostSnippetStatusInvalidOptionValue
			return
		}
		row.input.ServerAliveInterval = value
	case "serveralivecountmax":
		value, err := parseHostSnippetInt(value)
		if err != nil {
			row.status = HostSnippetStatusInvalidOptionValue
			return
		}
		row.input.ServerAliveCountMax = value
	case "connecttimeout":
		value, err := parseHostSnippetInt(value)
		if err != nil {
			row.status = HostSnippetStatusInvalidOptionValue
			return
		}
		row.input.ConnectTimeout = value
	}
}

func parseHostSnippetInt(value string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(value))
}

func parseHostSnippetBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "on":
		return true, true
	case "no", "false", "off":
		return false, true
	default:
		return false, false
	}
}
