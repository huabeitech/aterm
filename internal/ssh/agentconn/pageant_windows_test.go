package agentconn

import (
	"context"
	"errors"
	"testing"
	"time"
	"unsafe"
)

func TestPageantCopyDataMatchesWindowsABI(t *testing.T) {
	var message pageantCopyData
	size, offset := unsafe.Sizeof(message), unsafe.Offsetof(message.Bytes)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		if size != 24 || offset != 16 {
			t.Fatal("COPYDATASTRUCT 64-bit ABI differs", size, offset)
		}
	} else if size != 12 || offset != 8 {
		t.Fatal("COPYDATASTRUCT 32-bit ABI differs", size, offset)
	}
}
func TestPageantRejectsMalformedFramesWithoutContactingUserAgent(t *testing.T) {
	for _, frame := range [][]byte{nil, {0, 0, 0, 0}, {0, 0, 0, 4, 11}, make([]byte, pageantFrameLimit+1)} {
		if _, err := queryPageant(context.Background(), frame); err == nil {
			t.Fatal("invalid native IPC frame accepted")
		}
	}
}
func TestPageantBusyWorkerQueueObservesCancellation(t *testing.T) {
	// Occupy all native-call slots without sending a message or opening a map.
	for i := 0; i < cap(pageantSlots); i++ {
		pageantSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(pageantSlots); i++ {
			<-pageantSlots
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := queryPageant(ctx, []byte{0, 0, 0, 1, 11}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("queued IPC ignored cancellation", err)
	}
}
