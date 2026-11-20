//go:build darwin && !mas

package platform

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <mach/message.h>
#include <stdlib.h>

static CFUserNotificationRef atermCreateAgentDialog(
	const char *title, const char *message, const char *reject, const char *allow, const char *always,
	CFTimeInterval timeout, SInt32 *status
) {
	const void *keys[] = {
		kCFUserNotificationAlertHeaderKey, kCFUserNotificationAlertMessageKey,
		kCFUserNotificationDefaultButtonTitleKey, kCFUserNotificationAlternateButtonTitleKey,
		kCFUserNotificationOtherButtonTitleKey, kCFUserNotificationIconURLKey
	};
	CFBundleRef bundle = CFBundleGetMainBundle();
	CFURLRef icon = bundle == NULL ? NULL :
		CFBundleCopyResourceURL(bundle, CFSTR("iconfile"), CFSTR("icns"), NULL);
	const void *values[] = {
		CFStringCreateWithCString(NULL, title, kCFStringEncodingUTF8),
		CFStringCreateWithCString(NULL, message, kCFStringEncodingUTF8),
		CFStringCreateWithCString(NULL, reject, kCFStringEncodingUTF8),
		CFStringCreateWithCString(NULL, allow, kCFStringEncodingUTF8),
		CFStringCreateWithCString(NULL, always, kCFStringEncodingUTF8), icon
	};
	CFIndex count = icon == NULL ? 5 : 6;
	CFDictionaryRef options = CFDictionaryCreate(NULL, keys, values, count,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFUserNotificationRef dialog = CFUserNotificationCreate(NULL, timeout,
		kCFUserNotificationPlainAlertLevel, status, options);
	CFRelease(options);
	for (CFIndex i = 0; i < count; i++) CFRelease(values[i]);
	return dialog;
}

static Boolean atermReadLanguage(CFArrayRef languages, CFIndex index, char *buffer, CFIndex size) {
	return CFStringGetCString(CFArrayGetValueAtIndex(languages, index), buffer, size, kCFStringEncodingUTF8);
}
*/
import "C"

import (
	"context"
	"fmt"
	"time"
	"unsafe"
)

func PreferredLanguages() []string {
	languages := C.CFLocaleCopyPreferredLanguages()
	if languages == 0 {
		return nil
	}
	defer C.CFRelease(C.CFTypeRef(languages))
	count := C.CFArrayGetCount(languages)
	result := make([]string, 0, int(count))
	for index := C.CFIndex(0); index < count; index++ {
		var buffer [256]C.char
		if C.atermReadLanguage(languages, index, &buffer[0], C.CFIndex(len(buffer))) != 0 {
			result = append(result, C.GoString(&buffer[0]))
		}
	}
	return result
}

type AgentApproval int

const (
	AgentDenied AgentApproval = iota
	AgentAllowedOnce
	AgentAllowedAlways
)

// ConfirmAgentConnection presents a system dialog independently of app windows.
func ConfirmAgentConnection(ctx context.Context, title, message, reject, allow, always string, localeChanged func() bool) (AgentApproval, error) {
	if ctx.Err() != nil {
		return AgentDenied, nil
	}
	nativeTitle, nativeMessage := C.CString(title), C.CString(message)
	nativeReject, nativeAllow := C.CString(reject), C.CString(allow)
	nativeAlways := C.CString(always)
	var timeout C.CFTimeInterval
	if deadline, ok := ctx.Deadline(); ok {
		timeout = C.CFTimeInterval(max(time.Until(deadline).Seconds(), 0.001))
	}
	var status C.SInt32
	dialog := C.atermCreateAgentDialog(nativeTitle, nativeMessage, nativeReject, nativeAllow, nativeAlways, timeout, &status)
	C.free(unsafe.Pointer(nativeAlways))
	C.free(unsafe.Pointer(nativeTitle))
	C.free(unsafe.Pointer(nativeMessage))
	C.free(unsafe.Pointer(nativeReject))
	C.free(unsafe.Pointer(nativeAllow))
	if dialog == 0 {
		return AgentDenied, fmt.Errorf("could not present system dialog (status %d)", status)
	}
	defer C.CFRelease(C.CFTypeRef(dialog))
	// Short receives let cancellation and locale changes close the system dialog
	// without performing concurrent operations on the notification.
	for ctx.Err() == nil && !localeChanged() {
		var response C.CFOptionFlags
		status = C.CFUserNotificationReceiveResponse(dialog, 0.2, &response)
		switch status {
		case 0:
			if ctx.Err() == nil && !localeChanged() {
				switch response {
				case C.kCFUserNotificationAlternateResponse:
					return AgentAllowedOnce, nil
				case C.kCFUserNotificationOtherResponse:
					return AgentAllowedAlways, nil
				}
			}
			return AgentDenied, nil
		case C.MACH_RCV_TIMED_OUT:
			continue
		default:
			C.CFUserNotificationCancel(dialog)
			return AgentDenied, fmt.Errorf("could not receive system dialog response (status %d)", status)
		}
	}
	C.CFUserNotificationCancel(dialog)
	return AgentDenied, nil
}
