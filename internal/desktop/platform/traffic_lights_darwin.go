//go:build darwin && cgo

package platform

/*
#cgo LDFLAGS: -framework Cocoa
void SSHConnectAlignTrafficLights(double headerHeight);
*/
import "C"

// AlignTrafficLights keeps macOS's native window controls on the same centre
// line as the custom webview header. The implementation observes future native
// window layouts so resize and fullscreen transitions remain aligned too.
func AlignTrafficLights(headerHeight float64) {
	C.SSHConnectAlignTrafficLights(C.double(headerHeight))
}
