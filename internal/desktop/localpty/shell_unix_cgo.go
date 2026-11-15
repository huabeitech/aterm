//go:build !windows && cgo

package localpty

/*
#include <pwd.h>
#include <unistd.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>

// getpwuid_r uses the native account service (including Directory Services on
// macOS), just as Node/libuv's effective-user lookup does. No shell command or
// user's profile is run to discover the shell. Returned memory is caller-owned.
static char* aterm_account_shell(int *status) {
    size_t size = 16384;
    while (size <= 1024 * 1024) {
        char *buffer = malloc(size);
        if (!buffer) { *status = ENOMEM; return NULL; }
        struct passwd entry;
        struct passwd *result = NULL;
        int code;
        do { code = getpwuid_r(geteuid(), &entry, buffer, size, &result); } while (code == EINTR);
        if (code == ERANGE) { free(buffer); size *= 2; continue; }
        if (code != 0 || result == NULL) { free(buffer); *status = code ? code : ENOENT; return NULL; }
        char *shell = strdup(entry.pw_shell ? entry.pw_shell : "");
        free(buffer);
        *status = shell ? 0 : ENOMEM;
        return shell;
    }
    *status = ERANGE;
    return NULL;
}
*/
import "C"

import (
	"fmt"
	"syscall"
	"unsafe"
)

func accountShell() (string, error) {
	var status C.int
	value := C.aterm_account_shell(&status)
	if value == nil {
		return "", fmt.Errorf("read effective user's login shell: %w", syscall.Errno(status))
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value), nil
}
