package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <AppKit/AppKit.h>
#include <stdlib.h>

static char *atermCopyClipboardFilePaths(void) {
	@autoreleasepool {
		NSArray<NSURL *> *urls = [[NSPasteboard generalPasteboard]
			readObjectsForClasses:@[[NSURL class]]
			options:@{NSPasteboardURLReadingFileURLsOnlyKey: @YES}];
		NSMutableArray<NSString *> *paths = [NSMutableArray arrayWithCapacity:urls.count];
		for (NSURL *url in urls) {
			if (url.isFileURL && url.path.isAbsolutePath) {
				[paths addObject:url.path];
			}
		}
		NSData *json = [NSJSONSerialization dataWithJSONObject:paths options:0 error:NULL];
		if (!json) return NULL;
		char *result = calloc(json.length + 1, 1);
		if (result) [json getBytes:result length:json.length];
		return result;
	}
}
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func ClipboardFilePaths() []string {
	return application.InvokeSyncWithResult(func() []string {
		value := C.atermCopyClipboardFilePaths()
		if value == nil {
			return []string{}
		}
		defer C.free(unsafe.Pointer(value))
		paths := []string{}
		_ = json.Unmarshal([]byte(C.GoString(value)), &paths)
		return paths
	})
}
