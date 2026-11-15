//go:build !darwin || !cgo

package platform

import "github.com/wailsapp/wails/v3/pkg/application"

func SetInputMode(application.Window, string) {}
func StopInputModes()                         {}
