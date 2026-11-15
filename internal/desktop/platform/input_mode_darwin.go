//go:build darwin && cgo

package platform

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
void ATermSetInputMode(void *window, const char *mode);
void ATermStopInputModes(void);
*/
import "C"

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"sync/atomic"
	"unsafe"
)

var nativeInputModesActive atomic.Bool

func SetInputMode(window application.Window, mode string) {
	value := C.CString(mode)
	defer C.free(unsafe.Pointer(value))
	application.InvokeSync(func() { C.ATermSetInputMode(window.NativeWindow(), value); nativeInputModesActive.Store(true) })
}

func StopInputModes() {
	if !nativeInputModesActive.Swap(false) {
		return
	}
	application.InvokeSync(func() { C.ATermStopInputModes() })
}
