//go:build darwin

package service

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#include <Foundation/Foundation.h>
#include <stdlib.h>

static char* copyNSString(NSString *value) {
	if (value == nil) {
		return NULL;
	}
	return strdup([value UTF8String]);
}

static char* copyNSError(NSError *error) {
	if (error == nil) {
		return NULL;
	}
	return copyNSString([error localizedDescription]);
}

static char* createSecurityScopedBookmarkForPath(const char *path, char **errorOut) {
	@autoreleasepool {
		NSString *pathString = [NSString stringWithUTF8String:path];
		NSURL *url = [NSURL fileURLWithPath:pathString isDirectory:YES];
		NSError *error = nil;
		NSData *bookmark = [url bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
		                    includingResourceValuesForKeys:nil
		                                     relativeToURL:nil
		                                             error:&error];
		if (bookmark == nil) {
			*errorOut = copyNSError(error);
			return NULL;
		}
		NSString *encoded = [bookmark base64EncodedStringWithOptions:0];
		return copyNSString(encoded);
	}
}

static void* startSecurityScopedBookmarkAccess(const char *bookmarkText, char **resolvedPathOut, char **errorOut) {
	@autoreleasepool {
		NSString *encoded = [NSString stringWithUTF8String:bookmarkText];
		NSData *bookmark = [[NSData alloc] initWithBase64EncodedString:encoded options:0];
		if (bookmark == nil) {
			*errorOut = strdup("Invalid security-scoped bookmark data.");
			return NULL;
		}
		BOOL stale = NO;
		NSError *error = nil;
		NSURL *url = [NSURL URLByResolvingBookmarkData:bookmark
		                                       options:NSURLBookmarkResolutionWithSecurityScope
		                                 relativeToURL:nil
		                           bookmarkDataIsStale:&stale
		                                             error:&error];
		if (url == nil) {
			*errorOut = copyNSError(error);
			return NULL;
		}
		if (![url startAccessingSecurityScopedResource]) {
			*errorOut = strdup("Unable to start security-scoped resource access.");
			return NULL;
		}
		*resolvedPathOut = copyNSString([url path]);
		return (void*)CFBridgingRetain(url);
	}
}

static void stopSecurityScopedBookmarkAccess(void *handle) {
	if (handle == NULL) {
		return;
	}
	@autoreleasepool {
		NSURL *url = CFBridgingRelease(handle);
		[url stopAccessingSecurityScopedResource];
	}
}
*/
import "C"

import (
	"context"
	"errors"
	"unsafe"
)

func createSecurityScopedBookmark(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	var cError *C.char
	bookmark := C.createSecurityScopedBookmarkForPath(cPath, &cError)
	if bookmark == nil {
		defer freeCString(cError)
		return "", errors.New(goString(cError))
	}
	defer C.free(unsafe.Pointer(bookmark))
	return C.GoString(bookmark), nil
}

func startSecurityScopedAccess(ctx context.Context, path string, bookmark string) (string, func(), error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if bookmark == path {
		return path, func() {}, nil
	}
	if path == "" {
		path = bookmark
	}
	cBookmark := C.CString(bookmark)
	defer C.free(unsafe.Pointer(cBookmark))
	var cResolvedPath *C.char
	var cError *C.char
	handle := C.startSecurityScopedBookmarkAccess(cBookmark, &cResolvedPath, &cError)
	if handle == nil {
		defer freeCString(cError)
		return "", nil, errors.New(goString(cError))
	}
	resolvedPath := C.GoString(cResolvedPath)
	freeCString(cResolvedPath)
	return resolvedPath, func() {
		C.stopSecurityScopedBookmarkAccess(handle)
	}, nil
}

func goString(value *C.char) string {
	if value == nil {
		return "security-scoped bookmark operation failed"
	}
	return C.GoString(value)
}

func freeCString(value *C.char) {
	if value != nil {
		C.free(unsafe.Pointer(value))
	}
}
