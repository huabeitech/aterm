package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	sshconfig "ssh-connect/internal/ssh"
)

// ExportOpenSSHConfig 将当前 Host 数据导出为供外部 OpenSSH 客户端使用的配置快照。
// 该方法只允许由设置页面的手动导出操作触发；Host 增删改、批量导入和同步操作
// 都不会自动更新 ~/.ssh/config 或应用托管的生成配置。
func (s *SSHConfigService) ExportOpenSSHConfig(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.openSSHExportMu.Lock()
	defer s.openSSHExportMu.Unlock()
	hosts, err := s.store.ListHosts()
	if err != nil {
		return err
	}
	exportable, err := toSSHHosts(hosts)
	if err != nil {
		return err
	}
	rendered := sshconfig.RenderConfig(exportable)
	return s.withSSHDirectory(ctx, func(paths sshPaths) error {
		if err := ensureSSHPaths(paths); err != nil {
			return err
		}
		if err := s.atomicWriteFile(paths.generatedConfigPath, []byte(rendered), 0o600); err != nil {
			return fmt.Errorf("write OpenSSH snapshot: %w", err)
		}
		config, err := os.ReadFile(paths.configPath)
		if err != nil {
			return fmt.Errorf("OpenSSH snapshot was created at %s, but the SSH config could not be read: %w", paths.generatedConfigPath, err)
		}
		next := normalizeIncludeLine(string(config))
		if next == string(config) {
			return nil
		}
		backupPath := filepath.Join(filepath.Dir(paths.configPath), "config.ssh-connect."+time.Now().UTC().Format("20060102150405")+".bak")
		if err := s.atomicWriteFile(backupPath, config, 0o600); err != nil {
			return fmt.Errorf("OpenSSH snapshot was created at %s, but the SSH config backup could not be written: %w", paths.generatedConfigPath, err)
		}
		if err := s.atomicWriteFile(paths.configPath, []byte(next), 0o600); err != nil {
			return fmt.Errorf("OpenSSH snapshot was created at %s, but the Include line could not be updated: %w", paths.generatedConfigPath, err)
		}
		return nil
	})
}

// OpenSSHExportStatus 只在设置页面展示导出状态时检查外部 OpenSSH 文件。
// 应用启动和普通状态刷新不得调用该方法，避免把可选导出能力变成启动依赖。
func (s *SSHConfigService) OpenSSHExportStatus(ctx context.Context) (OpenSSHExportStatus, error) {
	if err := ctx.Err(); err != nil {
		return OpenSSHExportStatus{}, err
	}
	var status OpenSSHExportStatus
	err := s.withSSHDirectory(ctx, func(paths sshPaths) error {
		status.SSHConfigPath = paths.configPath
		status.GeneratedConfigPath = paths.generatedConfigPath
		includeConfigured, err := hasIncludeLine(paths.configPath)
		if err != nil {
			return err
		}
		status.IncludeConfigured = includeConfigured
		info, err := os.Stat(paths.generatedConfigPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		status.SnapshotExists = !info.IsDir()
		if status.SnapshotExists {
			status.LastExportedAt = info.ModTime().UTC().Format(time.RFC3339)
		}
		return nil
	})
	return status, err
}

func (s *SSHConfigService) AuthorizeSSHDirectory(ctx context.Context, path string) (AppSettings, error) {
	if err := ctx.Err(); err != nil {
		return AppSettings{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return AppSettings{}, ErrSSHDirectoryAuthorizationRequired
	}
	if err := s.ensureExpectedSSHDirectory(path); err != nil {
		return AppSettings{}, err
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		return AppSettings{}, err
	}
	bookmark, err := createSecurityScopedBookmark(ctx, path)
	if err != nil {
		return AppSettings{}, err
	}
	updated, err := s.store.UpdateSettings(AppSettingsInput{
		Theme:                          settings.Theme,
		TerminalTheme:                  settings.TerminalTheme,
		DefaultHome:                    settings.DefaultHome,
		CommandPaletteShortcut:         settings.CommandPaletteShortcut,
		NewTerminalShortcut:            settings.NewTerminalShortcut,
		CloseTerminalShortcut:          settings.CloseTerminalShortcut,
		PreviousTerminalShortcut:       settings.PreviousTerminalShortcut,
		NextTerminalShortcut:           settings.NextTerminalShortcut,
		MoveTabLeftShortcut:            settings.MoveTabLeftShortcut,
		MoveTabRightShortcut:           settings.MoveTabRightShortcut,
		TerminalFontSize:               settings.TerminalFontSize,
		TerminalWordNavigation:         settings.TerminalWordNavigation,
		SplitVerticalShortcut:          settings.SplitVerticalShortcut,
		SplitHorizontalShortcut:        settings.SplitHorizontalShortcut,
		SSHDirectoryPath:               path,
		SSHDirectoryBookmark:           bookmark,
		SSHDirectoryBookmarkCreatedAt:  time.Now().UTC().Format(time.RFC3339),
		SyncDirectoryPath:              settings.SyncDirectoryPath,
		SyncDirectoryBookmark:          settings.SyncDirectoryBookmark,
		SyncDirectoryBookmarkCreatedAt: settings.SyncDirectoryBookmarkCreatedAt,
	})
	if err != nil {
		return AppSettings{}, err
	}
	return updated, nil
}

func (s *SSHConfigService) ensureExpectedSSHDirectory(path string) error {
	expected, err := s.expectedSSHDir()
	if err != nil {
		return err
	}
	cleanSelected, err := canonicalPath(path)
	if err != nil {
		return err
	}
	cleanExpected, err := canonicalPath(expected)
	if err != nil {
		return err
	}
	if cleanSelected != cleanExpected {
		return fmt.Errorf("%w: select %s", ErrUnexpectedSSHDirectory, cleanExpected)
	}
	return os.MkdirAll(cleanExpected, 0o700)
}

func (s *SSHConfigService) expectedSSHDir() (string, error) {
	if strings.TrimSpace(s.expectedSSHDirectory) != "" {
		return s.expectedSSHDirectory, nil
	}
	if strings.TrimSpace(s.sshDir) != "" {
		return s.sshDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh"), nil
}

func canonicalPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrSSHDirectoryAuthorizationRequired
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

type sshPaths struct {
	sshDir              string
	configPath          string
	generatedConfigPath string
}

func (s *SSHConfigService) settingsSSHDirectoryPath() string {
	if s.store == nil {
		return ""
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		return ""
	}
	return settings.SSHDirectoryPath
}

func (s *SSHConfigService) withSSHDirectory(ctx context.Context, fn func(paths sshPaths) error) error {
	paths, done, err := s.authorizedSSHPaths(ctx)
	if err != nil {
		return err
	}
	defer done()
	return fn(paths)
}

func (s *SSHConfigService) authorizedSSHPaths(ctx context.Context) (sshPaths, func(), error) {
	if s.store == nil {
		return sshPaths{}, nil, errors.New("host store is required")
	}
	if !macAppStoreBuild {
		sshDir := strings.TrimSpace(s.sshDir)
		if sshDir == "" {
			var err error
			sshDir, err = s.expectedSSHDir()
			if err != nil {
				return sshPaths{}, nil, err
			}
		}
		configPath := strings.TrimSpace(s.configPath)
		if configPath == "" {
			configPath = filepath.Join(sshDir, "config")
		}
		generatedConfigPath := strings.TrimSpace(s.generatedConfigPath)
		if generatedConfigPath == "" {
			generatedConfigPath = filepath.Join(sshDir, "config.d", "ssh-connect.generated.conf")
		}
		return sshPaths{
			sshDir:              sshDir,
			configPath:          configPath,
			generatedConfigPath: generatedConfigPath,
		}, func() {}, nil
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		return sshPaths{}, nil, err
	}
	sshDir := strings.TrimSpace(settings.SSHDirectoryPath)
	bookmark := strings.TrimSpace(settings.SSHDirectoryBookmark)
	if sshDir == "" || bookmark == "" {
		return sshPaths{}, nil, ErrSSHDirectoryAuthorizationRequired
	}
	if err := s.ensureExpectedSSHDirectory(sshDir); err != nil {
		return sshPaths{}, nil, fmt.Errorf("%w: %v", ErrSSHDirectoryAuthorizationRequired, err)
	}
	resolvedPath, done, err := startSecurityScopedAccess(ctx, sshDir, bookmark)
	if err != nil {
		return sshPaths{}, nil, fmt.Errorf("%w: %v", ErrSSHDirectoryAuthorizationRequired, err)
	}
	if resolvedPath == "" {
		done()
		return sshPaths{}, nil, ErrSSHDirectoryAuthorizationRequired
	}
	return sshPaths{
		sshDir:              resolvedPath,
		configPath:          filepath.Join(resolvedPath, "config"),
		generatedConfigPath: filepath.Join(resolvedPath, "config.d", "ssh-connect.generated.conf"),
	}, done, nil
}

func ensureSSHPaths(paths sshPaths) error {
	if paths.sshDir == "" {
		return ErrSSHDirectoryAuthorizationRequired
	}
	if err := os.MkdirAll(paths.sshDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paths.generatedConfigPath), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(paths.configPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

func hasIncludeLine(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return containsEffectiveIncludeLine(string(data)), nil
}

func containsEffectiveIncludeLine(config string) bool {
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == IncludeLine {
			return true
		}
		if strings.HasPrefix(trimmed, "Host ") || strings.HasPrefix(trimmed, "Match ") {
			return false
		}
	}
	return false
}

func normalizeIncludeLine(config string) string {
	lines := strings.Split(strings.TrimRight(config, "\n"), "\n")
	out := make([]string, 0, len(lines)+3)
	insertAt := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == IncludeLine {
			continue
		}
		if trimmed == "# Added by aTerm" {
			continue
		}
		if insertAt == -1 && (strings.HasPrefix(trimmed, "Host ") || strings.HasPrefix(trimmed, "Match ")) {
			insertAt = len(out)
		}
		out = append(out, line)
	}
	includeBlock := []string{"# Added by aTerm", IncludeLine}
	if insertAt == -1 {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, includeBlock...)
	} else {
		block := append([]string{}, includeBlock...)
		if insertAt < len(out) && strings.TrimSpace(out[insertAt]) != "" {
			block = append(block, "")
		}
		out = append(out[:insertAt], append(block, out[insertAt:]...)...)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

func toSSHHosts(hosts []Host) ([]sshconfig.Host, error) {
	byID := make(map[string]Host, len(hosts))
	for _, host := range hosts {
		byID[host.ID] = host
	}
	out := make([]sshconfig.Host, 0, len(hosts))
	for _, host := range hosts {
		if host.Alias == "" {
			continue
		}
		proxyJump, err := resolvedProxyJump(host, byID)
		if err != nil {
			return nil, err
		}
		out = append(out, sshconfig.Host{
			Alias:               host.Alias,
			HostName:            host.HostName,
			User:                host.User,
			Port:                host.Port,
			IdentityFile:        host.IdentityFile,
			ProxyJump:           proxyJump,
			ForwardAgent:        host.ForwardAgent,
			Compression:         host.Compression,
			ServerAliveInterval: host.ServerAliveInterval,
			ServerAliveCountMax: host.ServerAliveCountMax,
			ConnectTimeout:      host.ConnectTimeout,
		})
	}
	return out, nil
}

var jumpEndpointToken = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`)

func resolvedProxyJump(host Host, byID map[string]Host) (string, error) {
	jumps := []string{}
	seen := map[string]bool{host.ID: true}
	current := host
	for {
		jump, ok := byID[current.JumpHostID]
		if !ok {
			if current.ProxyJump != "" {
				jumps = append(jumps, current.ProxyJump)
			}
			break
		}
		if seen[jump.ID] {
			return "", fmt.Errorf("OpenSSH ProxyJump cycle for %q", host.DisplayName())
		}
		if jump.Alias != "" {
			jumps = append(jumps, jump.Alias)
			break
		}
		seen[jump.ID] = true
		address := strings.Trim(jump.HostName, "[]")
		if (!jumpEndpointToken.MatchString(address) && net.ParseIP(address) == nil) ||
			(jump.User != "" && !jumpEndpointToken.MatchString(jump.User)) {
			return "", fmt.Errorf("invalid OpenSSH ProxyJump endpoint for %q", jump.DisplayName())
		}
		endpoint := net.JoinHostPort(address, strconv.Itoa(resolvedSSHPort(jump.Port)))
		if jump.User != "" {
			endpoint = jump.User + "@" + endpoint
		}
		jumps = append(jumps, endpoint)
		current = jump
	}
	// ProxyJump lists the outermost jump first.
	slices.Reverse(jumps)
	return strings.Join(jumps, ","), nil
}
