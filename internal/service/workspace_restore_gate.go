package service

import (
	"context"
	"sync"
)

// workspaceRestoreGate coordinates restore commits for one application service.
// Validation/confirmation stay outside it; callers waiting to install may cancel.
// It is deliberately separate from encryptedSyncMu, which session activation and
// credential persistence acquire during installation.
type workspaceRestoreGate struct {
	once     sync.Once
	occupied chan struct{}
}

func (g *workspaceRestoreGate) acquire(ctx context.Context) (func(), error) {
	g.once.Do(func() { g.occupied = make(chan struct{}, 1) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case g.occupied <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-g.occupied
		return nil, err
	}
	var released sync.Once
	return func() { released.Do(func() { <-g.occupied }) }, nil
}
