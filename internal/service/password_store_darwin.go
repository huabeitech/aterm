//go:build darwin

package service

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static CFStringRef cfstr(const char *value) {
	return CFStringCreateWithCString(NULL, value, kCFStringEncodingUTF8);
}

static OSStatus password_set(const char *service_name, const char *account, const char *password) {
	CFStringRef service = cfstr(service_name), key = cfstr(account);
	CFDataRef data = CFDataCreate(NULL, (const UInt8 *)password, (CFIndex)strlen(password));
	const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount };
	const void *values[] = { kSecClassGenericPassword, service, key };
	CFDictionaryRef query = CFDictionaryCreate(NULL, keys, values, 3, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	const void *updateKeys[] = { kSecValueData, kSecAttrAccessible };
	const void *updateValues[] = { data, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly };
	CFDictionaryRef update = CFDictionaryCreate(NULL, updateKeys, updateValues, 2, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus status = SecItemUpdate(query, update);
	if (status == errSecItemNotFound) {
		const void *addKeys[] = { kSecClass, kSecAttrService, kSecAttrAccount, kSecValueData, kSecAttrAccessible };
		const void *addValues[] = { kSecClassGenericPassword, service, key, data, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly };
		CFDictionaryRef add = CFDictionaryCreate(NULL, addKeys, addValues, 5, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
		status = SecItemAdd(add, NULL);
		CFRelease(add);
	}
	CFRelease(update); CFRelease(query); CFRelease(data); CFRelease(key); CFRelease(service);
	return status;
}

static OSStatus password_get(const char *service_name, const char *account, char **out, size_t *length) {
	CFStringRef service = cfstr(service_name), key = cfstr(account);
	const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount, kSecReturnData };
	const void *values[] = { kSecClassGenericPassword, service, key, kCFBooleanTrue };
	CFDictionaryRef query = CFDictionaryCreate(NULL, keys, values, 4, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	if (status == errSecSuccess) {
		CFDataRef data = (CFDataRef)result;
		*length = (size_t)CFDataGetLength(data);
		*out = malloc(*length + 1);
		memcpy(*out, CFDataGetBytePtr(data), *length);
		(*out)[*length] = 0;
		CFRelease(result);
	}
	CFRelease(query); CFRelease(key); CFRelease(service);
	return status;
}

static OSStatus password_delete(const char *service_name, const char *account) {
	CFStringRef service = cfstr(service_name), key = cfstr(account);
	const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount };
	const void *values[] = { kSecClassGenericPassword, service, key };
	CFDictionaryRef query = CFDictionaryCreate(NULL, keys, values, 3, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus status = SecItemDelete(query);
	CFRelease(query); CFRelease(key); CFRelease(service);
	return status;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"unsafe"
)

const errSecItemNotFound = -25300
const errSecUserCanceled = -128
const errSecInteractionNotAllowed = -25308
const errSecAuthFailed = -25293
const errSecNotAvailable = -25291

type keychainPasswordStore struct {
	wailsContext context.Context
}

func newPasswordStore(ctx context.Context) PasswordStore {
	return keychainPasswordStore{wailsContext: ctx}
}

func (store keychainPasswordStore) Get(hostID string) (string, error) {
	service := C.CString(keychainServiceName())
	defer C.free(unsafe.Pointer(service))
	account := C.CString(hostID)
	defer C.free(unsafe.Pointer(account))
	var value *C.char
	var length C.size_t
	status := C.password_get(service, account, &value, &length)
	store.traceKeychain("get", hostID, int(status))
	if status == errSecItemNotFound {
		return "", ErrPasswordNotFound
	}
	if status != 0 {
		return "", classifyKeychainStatus("read SSH password from Keychain", int(status))
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoStringN(value, C.int(length)), nil
}

func (store keychainPasswordStore) Set(hostID, password string) error {
	service := C.CString(keychainServiceName())
	account, value := C.CString(hostID), C.CString(password)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	defer C.free(unsafe.Pointer(value))
	if status := C.password_set(service, account, value); status != 0 {
		store.traceKeychain("set", hostID, int(status))
		err := classifyKeychainStatus("save SSH password to Keychain", int(status))
		if errors.Is(err, ErrKeychainInteractionNotAllowed) || errors.Is(err, ErrKeychainAccessDenied) {
			_ = store.Delete(hostID)
			if retryStatus := C.password_set(service, account, value); retryStatus == 0 {
				store.traceKeychain("set-retry", hostID, int(retryStatus))
				return nil
			} else {
				store.traceKeychain("set-retry", hostID, int(retryStatus))
				return classifyKeychainStatus("save SSH password to Keychain", int(retryStatus))
			}
		}
		return err
	}
	store.traceKeychain("set", hostID, 0)
	return nil
}

func (store keychainPasswordStore) Delete(hostID string) error {
	service := C.CString(keychainServiceName())
	defer C.free(unsafe.Pointer(service))
	account := C.CString(hostID)
	defer C.free(unsafe.Pointer(account))
	status := C.password_delete(service, account)
	store.traceKeychain("delete", hostID, int(status))
	if status == 0 || status == errSecItemNotFound {
		return nil
	}
	return classifyKeychainStatus("remove SSH password from Keychain", int(status))
}

func keychainServiceName() string {
	suffix := ""
	if developmentBuild {
		suffix = "-dev"
	}
	if macAppStoreBuild {
		return "TermSSH(MAS)" + suffix
	}
	return "TermSSH" + suffix
}

func (store keychainPasswordStore) traceKeychain(operation, account string, status int) {
	slog.InfoContext(store.wailsContext, "termssh-keychain", "operation", operation, "account", account, "status", status)
}

func classifyKeychainStatus(operation string, status int) error {
	switch status {
	case errSecUserCanceled:
		return fmt.Errorf("%w: %s status %d", ErrKeychainUserCanceled, operation, status)
	case errSecInteractionNotAllowed:
		return fmt.Errorf("%w: %s status %d", ErrKeychainInteractionNotAllowed, operation, status)
	case errSecAuthFailed:
		return fmt.Errorf("%w: %s status %d", ErrKeychainAccessDenied, operation, status)
	case errSecNotAvailable:
		return fmt.Errorf("%w: %s status %d", ErrPasswordStoreUnavailable, operation, status)
	default:
		return fmt.Errorf("%w: %s status %d", ErrPasswordStoreUnavailable, operation, status)
	}
}
