package service

import (
	"io/fs"

	"ssh-connect/internal/fsutil"
)

// These focused capability services preserve the existing AppService API via
// embedding while assigning each operation to one cohesive service. AppService
// remains the composition facade used by the Wails layer.
type SSHConfigService struct {
	*AppService
	atomicWriteFile func(string, []byte, fs.FileMode) error
}
type SettingsService struct{ *AppService }
type ConnectionService struct{ *AppService }
type SyncService struct{ *AppService }
type RemoteFileService struct{ *AppService }
type HostImportService struct{ *AppService }

func newSSHConfigService(app *AppService) *SSHConfigService {
	return &SSHConfigService{AppService: app, atomicWriteFile: fsutil.AtomicWriteFile}
}
