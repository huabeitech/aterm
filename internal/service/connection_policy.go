package service

import (
	"context"
)

// ConnectionPolicy centralizes build-variant capability gates.
type ConnectionPolicy struct {
	featuresAvailable bool
}

func NewConnectionPolicy(featuresAvailable bool) *ConnectionPolicy {
	return &ConnectionPolicy{featuresAvailable: featuresAvailable}
}

func (p *ConnectionPolicy) requireConnectionFeatures(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !p.featuresAvailable {
		return ErrConnectionFeaturesUnavailable
	}
	return nil
}
