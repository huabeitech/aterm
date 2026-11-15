package service

import (
	"context"
	"ssh-connect/internal/domain"
)

func (s *SettingsService) GetSettings(ctx context.Context) (AppSettings, error) {
	if err := ctx.Err(); err != nil {
		return AppSettings{}, err
	}
	return s.store.GetSettings()
}

func (s *SettingsService) UpdateSettings(ctx context.Context, input AppSettingsInput) (AppSettings, error) {
	if err := ctx.Err(); err != nil {
		return AppSettings{}, err
	}
	return s.store.UpdateSettings(input)
}

// Settings validation belongs to the domain so database, sync and UI updates
// share the same defaults and invariants.
func DefaultAppSettings() AppSettings { return domain.DefaultAppSettings() }

func NormalizeAppSettingsInput(input AppSettingsInput) (AppSettings, error) {
	return domain.NormalizeAppSettingsInput(input)
}
