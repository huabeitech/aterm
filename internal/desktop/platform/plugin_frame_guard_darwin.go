//go:build darwin && cgo

package platform

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdbool.h>
bool ATermInstallFrameMessageGuard(void *nativeWindow);
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// WKWebView exposes a readonly webkit.messageHandlers object in every frame.
// Filter at the public WKScriptMessageHandler boundary instead of pretending
// that JavaScript can remove that native capability.
func SecurePluginFrames(window application.Window) bool {
	if window == nil {
		return false
	}
	secured := false
	application.InvokeSync(func() { secured = bool(C.ATermInstallFrameMessageGuard(window.NativeWindow())) })
	return secured
}
