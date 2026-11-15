package db

import "ssh-connect/internal/domain"

func scanHost(scanner hostScanner) (domain.Host, error) {
	var host domain.Host
	var passwordAuth, forwardAgent, compression, favorite int
	err := scanner.Scan(
		&host.ID,
		&host.DirectoryID,
		&host.Alias,
		&host.HostName,
		&host.User,
		&host.Port,
		&host.IdentityFile,
		&passwordAuth,
		&host.ProxyJump,
		&host.JumpHostID,
		&forwardAgent,
		&compression,
		&host.ServerAliveInterval,
		&host.ServerAliveCountMax,
		&host.ConnectTimeout,
		&host.Remark,
		&favorite,
		&host.LastUsedAt,
		&host.CreatedAt,
		&host.UpdatedAt,
	)
	host.ForwardAgent = forwardAgent == 1
	host.PasswordAuth = passwordAuth == 1
	host.Compression = compression == 1
	host.Favorite = favorite == 1
	return host, err
}

func scanHostSortOrder(scanner hostScanner) (domain.HostSortOrder, error) {
	var sortOrder domain.HostSortOrder
	err := scanner.Scan(
		&sortOrder.ScopeType,
		&sortOrder.ScopeID,
		&sortOrder.HostID,
		&sortOrder.SortOrder,
		&sortOrder.CreatedAt,
		&sortOrder.UpdatedAt,
	)
	return sortOrder, err
}

func scanDirectory(scanner hostScanner) (domain.Directory, error) {
	var directory domain.Directory
	err := scanner.Scan(
		&directory.ID,
		&directory.ParentID,
		&directory.Name,
		&directory.SortOrder,
		&directory.CreatedAt,
		&directory.UpdatedAt,
	)
	return directory, err
}
