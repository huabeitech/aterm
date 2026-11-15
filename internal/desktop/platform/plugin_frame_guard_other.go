//go:build !darwin || !cgo

package platform

import "github.com/wailsapp/wails/v3/pkg/application"

// A platform bridge must enforce main-frame-only messages before hosting
// untrusted DOM. No silent JavaScript-only fallback is used.
func SecurePluginFrames(_ application.Window) bool { return false }
