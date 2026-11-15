package agentconn

// Pageant's legacy WM_COPYDATA/shared-memory framing is also documented by
// github.com/davidmz/go-pageant. This implementation adds bounded native calls,
// random mapping names and current-user-only ownership/permissions.
import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pageantUser32        = windows.NewLazySystemDLL("user32.dll")
	findPageantWindow    = pageantUser32.NewProc("FindWindowW")
	sendPageantMessage   = pageantUser32.NewProc("SendMessageTimeoutW")
	pageantSetLastError  = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetLastError")
	pageantWindowProcess = pageantUser32.NewProc("GetWindowThreadProcessId")
	pageantSlots         = make(chan struct{}, 8)
)

type pageantCopyData struct {
	Data  uintptr
	Size  uint32
	Bytes unsafe.Pointer
}

func pageantWindow() uintptr {
	name, _ := windows.UTF16PtrFromString("Pageant")
	window, _, _ := findPageantWindow.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	return window
}
func connectPageant() (net.Conn, error) {
	if pageantWindow() == 0 {
		return nil, ErrUnavailable
	}
	return &pageantConnection{newQueryConnection(queryPageant)}, nil
}
func queryPageant(ctx context.Context, frame []byte) ([]byte, error) {
	if !validAgentFrame(frame) {
		return nil, errors.New("invalid Pageant agent request frame")
	}
	select {
	case pageantSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-pageantSlots }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	window := pageantWindow()
	if window == 0 {
		return nil, ErrUnavailable
	}
	var processID uint32
	pageantWindowProcess.Call(window, uintptr(unsafe.Pointer(&processID)))
	// The documented timeout does not apply to a window on the sender's own
	// queue. aTerm never hosts Pageant, so refuse a same-process impersonation.
	if processID == 0 || processID == windows.GetCurrentProcessId() {
		return nil, errors.New("invalid Pageant window owner")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	name := "PageantRequest" + hex.EncodeToString(nonce[:])
	wideName, _ := windows.UTF16PtrFromString(name)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sid := user.User.Sid.String()
	descriptor, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;;GA;;;" + sid + ")")
	if err != nil {
		return nil, err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	// Keep the protocol reference's extra length-prefix space in the native
	// mapping even though the client accepts at most pageantFrameLimit bytes.
	const mappingSize = pageantFrameLimit + 4
	mapping, err := windows.CreateFileMapping(windows.InvalidHandle, &attributes, windows.PAGE_READWRITE, 0, mappingSize, wideName)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(wideName)
	if mapping != 0 {
		defer windows.CloseHandle(mapping)
	}
	if err != nil {
		return nil, err
	}
	address, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_WRITE|windows.FILE_MAP_READ, 0, 0, mappingSize)
	if err != nil {
		return nil, err
	}
	defer windows.UnmapViewOfFile(address)
	shared := unsafe.Slice((*byte)(unsafe.Pointer(address)), mappingSize)
	copy(shared, frame)
	nameBytes := append([]byte(name), 0)
	message := pageantCopyData{Data: 0x804e50ba, Size: uint32(len(nameBytes)), Bytes: unsafe.Pointer(&nameBytes[0])}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(pageantQueryTimeout)
	}
	remaining := min(time.Until(deadline), pageantQueryTimeout)
	if remaining <= 0 {
		return nil, context.DeadlineExceeded
	}
	milliseconds := uint32((remaining + time.Millisecond - 1) / time.Millisecond)
	if milliseconds == 0 {
		milliseconds = 1
	}
	var accepted uintptr
	// SMTO_BLOCK | SMTO_ABORTIFHUNG: use only the target Pageant window, never
	// broadcast. The per-operation context bounds queued work and native wait.
	if err = sendPageantMessage.Find(); err != nil {
		return nil, err
	}
	if err = pageantSetLastError.Find(); err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	pageantSetLastError.Call(0)
	result, _, callErr := sendPageantMessage.Call(window, 74, 0, uintptr(unsafe.Pointer(&message)), 3, uintptr(milliseconds), uintptr(unsafe.Pointer(&accepted)))
	runtime.UnlockOSThread()
	runtime.KeepAlive(message)
	runtime.KeepAlive(nameBytes)
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if result == 0 {
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return nil, fmt.Errorf("Pageant IPC: %w", callErr)
		}
		return nil, errors.New("Pageant did not respond before the IPC timeout")
	}
	if accepted == 0 {
		return nil, errors.New("Pageant rejected the agent request")
	}
	size := binary.BigEndian.Uint32(shared[:4])
	if size == 0 || size > pageantFrameLimit-4 {
		return nil, errors.New("Pageant response exceeds its frame limit")
	}
	response := append([]byte(nil), shared[:4+int(size)]...)
	clear(shared)
	return response, nil
}
