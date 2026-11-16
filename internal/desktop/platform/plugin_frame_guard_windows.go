//go:build windows

package platform

import "github.com/wailsapp/wails/v3/pkg/application"

// Wails subscribes to CoreWebView2.WebMessageReceived, which delivers
// top-level messages. It does not subscribe to the frame-specific event.
// Plugin documents still require the sandbox and CSP in the asset handler.
func SecurePluginFrames(window application.Window) bool {
	webview, ok := window.(*application.WebviewWindow)
	return ok && webview.NativeWindow() != nil
}
