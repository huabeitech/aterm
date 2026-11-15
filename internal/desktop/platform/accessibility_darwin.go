//go:build darwin && cgo

package platform

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdbool.h>
#include <stdint.h>
bool ATermVoiceOverEnabled(void);
void *ATermObserveVoiceOver(uintptr_t handle);
void ATermStopObservingVoiceOver(void *observer);
*/
import "C"

import (
	"runtime/cgo"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func AccessibilityEnabled() bool {
	enabled := false
	application.InvokeSync(func() { enabled = bool(C.ATermVoiceOverEnabled()) })
	return enabled
}

//export aTermVoiceOverChanged
func aTermVoiceOverChanged(handle C.uintptr_t, enabled C.bool) {
	cgo.Handle(handle).Value().(func(bool))(bool(enabled))
}

func ObserveAccessibility(changed func(bool)) func() {
	handle := cgo.NewHandle(changed)
	var observer unsafe.Pointer
	application.InvokeSync(func() { observer = C.ATermObserveVoiceOver(C.uintptr_t(handle)) })
	var once sync.Once
	return func() {
		once.Do(func() {
			application.InvokeSync(func() { C.ATermStopObservingVoiceOver(observer) })
			handle.Delete()
		})
	}
}
