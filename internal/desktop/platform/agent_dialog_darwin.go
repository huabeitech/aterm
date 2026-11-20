//go:build darwin && !mas

package platform

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <mach/message.h>
#include <stdlib.h>

static CFUserNotificationRef atermCreateAgentDialog(
	const char *title, const char *message, const char *reject, const char *allow,
	CFTimeInterval timeout, SInt32 *status
) {
	const void *keys[] = {
		kCFUserNotificationAlertHeaderKey, kCFUserNotificationAlertMessageKey,
		kCFUserNotificationDefaultButtonTitleKey, kCFUserNotificationAlternateButtonTitleKey,
		kCFUserNotificationIconURLKey
	};
	CFBundleRef bundle = CFBundleGetMainBundle();
	CFURLRef icon = bundle == NULL ? NULL :
		CFBundleCopyResourceURL(bundle, CFSTR("iconfile"), CFSTR("icns"), NULL);
	const void *values[] = {
		CFStringCreateWithCString(NULL, title, kCFStringEncodingUTF8),
		CFStringCreateWithCString(NULL, message, kCFStringEncodingUTF8),
		CFStringCreateWithCString(NULL, reject, kCFStringEncodingUTF8),
		CFStringCreateWithCString(NULL, allow, kCFStringEncodingUTF8),
		icon
	};
	CFIndex count = icon == NULL ? 4 : 5;
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

// ConfirmAgentConnection presents a system dialog independently of app windows.
func ConfirmAgentConnection(ctx context.Context, title, message, reject, allow string, localeChanged func() bool) (bool, error) {
	if ctx.Err() != nil {
		return false, nil
	}
	nativeTitle, nativeMessage := C.CString(title), C.CString(message)
	nativeReject, nativeAllow := C.CString(reject), C.CString(allow)
	var timeout C.CFTimeInterval
	if deadline, ok := ctx.Deadline(); ok {
		timeout = C.CFTimeInterval(max(time.Until(deadline).Seconds(), 0.001))
	}
	var status C.SInt32
	dialog := C.atermCreateAgentDialog(nativeTitle, nativeMessage, nativeReject, nativeAllow, timeout, &status)
	C.free(unsafe.Pointer(nativeTitle))
	C.free(unsafe.Pointer(nativeMessage))
	C.free(unsafe.Pointer(nativeReject))
	C.free(unsafe.Pointer(nativeAllow))
	if dialog == 0 {
		return false, fmt.Errorf("could not present system dialog (status %d)", status)
	}
	defer C.CFRelease(C.CFTypeRef(dialog))
	// Short receives let cancellation and locale changes close the system dialog
	// without performing concurrent operations on the notification.
	for ctx.Err() == nil && !localeChanged() {
		var response C.CFOptionFlags
		status = C.CFUserNotificationReceiveResponse(dialog, 0.2, &response)
		switch status {
		case 0:
			return ctx.Err() == nil && !localeChanged() && response == C.kCFUserNotificationAlternateResponse, nil
		case C.MACH_RCV_TIMED_OUT:
			continue
		default:
			C.CFUserNotificationCancel(dialog)
			return false, fmt.Errorf("could not receive system dialog response (status %d)", status)
		}
	}
	C.CFUserNotificationCancel(dialog)
	return false, nil
}
