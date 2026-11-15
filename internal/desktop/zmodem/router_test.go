package zmodem

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func receiveTranscript(t *testing.T, payload []byte) []byte {
	t.Helper()
	wire := mustHeader(t, ZRQINIT, 0, Hex)
	wire = append(wire, offerWire(t, len(payload), Binary32)...)
	wire = append(wire, mustHeader(t, ZDATA, 0, Binary32)...)
	wire = append(wire, mustPacket(t, payload, ZCRCE, Binary32)...)
	wire = append(wire, mustHeader(t, ZEOF, uint32(len(payload)), Hex)...)
	wire = append(wire, mustHeader(t, ZFIN, 0, Hex)...)
	return append(wire, 'O', 'O')
}
func TestRouterSeparatesProtocolFromCanonicalTerminalOutput(t *testing.T) {
	var text bytes.Buffer
	file := &stagedDownload{}
	var last Progress
	router := NewRouter(context.Background(), RouterOptions{Output: func(data []byte) { text.Write(data) }, Write: func([]byte) error { return nil }, Offer: func(context.Context, FileOffer) (Download, error) { return file, nil }, Progress: func(value Progress) { last = value }})
	payload := append([]byte("binary\x00"), mustHeader(t, ZRQINIT, 0, Hex)...)
	wire := append([]byte("before ☆ *ordinary*\r\n"), receiveTranscript(t, payload)...)
	wire = append(wire, []byte("after 🌏")...)
	for len(wire) > 0 {
		size := min(3, len(wire))
		router.Consume(wire[:size])
		wire = wire[size:]
	}
	if got := text.String(); got != "before ☆ *ordinary*\r\nafter 🌏" {
		t.Fatalf("protocol leaked into terminal output: %q", got)
	}
	if !bytes.Equal(file.Bytes(), payload) || file.committed != 1 || last.State != "completed" || router.Busy() {
		t.Fatal("router did not complete receive session", last)
	}
	router.Consume([]byte("*"))
	router.Close()
	if !strings.HasSuffix(text.String(), "🌏*") {
		t.Fatal("trailing terminal prefix was lost")
	}
}
func TestRouterBoundsIncompleteDetectionAndPreservesNonTransferHeaders(t *testing.T) {
	var text bytes.Buffer
	router := NewRouter(context.Background(), RouterOptions{Output: func(data []byte) { text.Write(data) }})
	wire := append([]byte{'*', ZDLE, byte(Binary16)}, bytes.Repeat([]byte{0x11}, 10000)...)
	for len(wire) > 0 {
		size := min(31, len(wire))
		router.Consume(wire[:size])
		wire = wire[size:]
		if len(router.pending) > 256 {
			t.Fatal("unbounded incomplete header detection")
		}
	}
	router.Close()
	if text.Len() != 10003 {
		t.Fatal("incomplete detection lost ordinary output", text.Len())
	}
}
func TestRouterCancelAndTimeoutInterruptSelection(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "timeout"}[timeout], func(t *testing.T) {
			started := make(chan struct{})
			progress := make(chan Progress, 16)
			duration := time.Second
			if timeout {
				duration = 30 * time.Millisecond
			}
			router := NewRouter(context.Background(), RouterOptions{Timeout: duration, Write: func([]byte) error { return nil }, Offer: func(ctx context.Context, _ FileOffer) (Download, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}, Progress: func(value Progress) { progress <- value }})
			done := make(chan struct{})
			wire := append(mustHeader(t, ZRQINIT, 0, Hex), offerWire(t, 4, Binary32)...)
			go func() { router.Consume(wire); close(done) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("selection did not start")
			}
			if !timeout {
				if err := router.Cancel(); err != nil {
					t.Fatal(err)
				}
				if router.Busy() {
					t.Fatal("cancel left input blocked")
				}
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("selection did not stop")
			}
			var last Progress
			for len(progress) > 0 {
				last = <-progress
			}
			if timeout {
				if last.State != "failed" || !strings.Contains(last.Error, "timed out") {
					t.Fatal("timeout lost its failure reason", last)
				}
			} else if last.State != "cancelled" || last.Error != "" {
				t.Fatal(last)
			}
			router.Close()
		})
	}
}

// A silent peer must not leave the terminal input guard active after timeout.
func TestRouterTimeoutReleasesInputWithoutAnotherPeerFrame(t *testing.T) {
	progress := make(chan Progress, 4)
	router := NewRouter(context.Background(), RouterOptions{
		Write:    func([]byte) error { return nil },
		Progress: func(value Progress) { progress <- value },
		Timeout:  10 * time.Millisecond,
	})
	defer router.Close()
	frame, err := EncodeHeader(Header{Type: ZRQINIT}, Hex)
	if err != nil {
		t.Fatal(err)
	}
	router.Consume(frame)
	if !router.Busy() {
		t.Fatal("handshake did not start receiving")
	}
	deadline := time.After(time.Second)
	for {
		select {
		case value := <-progress:
			if value.State != "failed" {
				continue
			}
			// Progress is emitted just before the timeout callback releases ownership.
			for router.Busy() {
				select {
				case <-deadline:
					t.Fatal("timeout kept input locked")
				default:
					runtime.Gosched()
				}
			}
			return
		case <-deadline:
			t.Fatal("transfer did not time out")
		}
	}
}
