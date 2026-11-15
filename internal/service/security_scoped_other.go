//go:build !darwin

package service

import "context"

func createSecurityScopedBookmark(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return path, nil
}

func startSecurityScopedAccess(ctx context.Context, path string, bookmark string) (string, func(), error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if path == "" {
		path = bookmark
	}
	return path, func() {}, nil
}
