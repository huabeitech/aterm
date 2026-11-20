package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <AppKit/AppKit.h>

static void atermPromoteToForeground(void) {
	[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
	[NSApp unhideWithoutActivation];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

func PromoteToForeground() {
	application.InvokeSync(func() { C.atermPromoteToForeground() })
}
