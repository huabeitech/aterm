package zmodem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type uploadSource struct {
	*bytes.Reader
	closed atomic.Int32
	err    error
}

func (s *uploadSource) Close() error { s.closed.Add(1); return nil }
func (s *uploadSource) ReadAt(p []byte, offset int64) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.Reader.ReadAt(p, offset)
}
func source(data []byte) *uploadSource { return &uploadSource{Reader: bytes.NewReader(data)} }
func uploadReady(t *testing.T, format Encoding) []byte {
	t.Helper()
	flags := byte(0x43)
	if format == Binary32 {
		flags |= 0x20
	}
	wire, err := EncodeHeader(Header{Type: ZRINIT, Data: [4]byte{0, 0, 0, flags}}, Hex)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}
func feedSend(t *testing.T, s *Sender, wire []byte) {
	t.Helper()
	for _, octet := range wire {
		if _, err := s.Feed([]byte{octet}); err != nil {
			t.Fatal(err)
		}
	}
}
func readSendHeader(t *testing.T, wire *bytes.Buffer, kind FrameType) Header {
	t.Helper()
	header, n, err := DecodeHeader(wire.Bytes())
	if err != nil || header.Type != kind {
		t.Fatalf("expected header %d, got %+v: %v; wire %x", kind, header, err, wire.Bytes())
	}
	wire.Next(n)
	return header
}
func readSendPacket(t *testing.T, wire *bytes.Buffer, format Encoding) Subpacket {
	t.Helper()
	packet, n, err := DecodeSubpacket(wire.Bytes(), format)
	if err != nil {
		t.Fatal(err)
	}
	wire.Next(n)
	return packet
}
func TestSenderResumesStreamsAcknowledgedWindowsAndFinishes(t *testing.T) {
	for _, format := range []Encoding{Binary16, Binary32} {
		t.Run(string(format), func(t *testing.T) {
			payload := make([]byte, 2*uploadWindow+17)
			for i := range payload {
				payload[i] = byte(i)
			}
			file := source(payload)
			var output bytes.Buffer
			var final Progress
			sender := NewSender(context.Background(), SendOptions{Write: func(data []byte) error { output.Write(data); return nil }, Select: func(context.Context) ([]Upload, error) {
				return []Upload{{Offer: FileOffer{Name: "fixture.bin", Size: int64(len(payload)), SizeKnown: true}, Reader: file}}, nil
			}, Progress: func(value Progress) { final = value }})
			feedSend(t, sender, uploadReady(t, format))
			readSendHeader(t, &output, ZFILE)
			offer := readSendPacket(t, &output, format)
			info, err := parseOffer(offer.Payload)
			if err != nil || info.Size != int64(len(payload)) {
				t.Fatal(info, err)
			}
			offset := uint32(73)
			feedSend(t, sender, mustHeader(t, ZRPOS, offset, Hex))
			if header := readSendHeader(t, &output, ZDATA); header.Offset() != offset {
				t.Fatal(header)
			}
			var received bytes.Buffer
			for {
				packet := readSendPacket(t, &output, format)
				received.Write(packet.Payload)
				offset += uint32(len(packet.Payload))
				if packet.End == ZCRCE {
					break
				}
				if (packet.End != ZCRCQ && packet.End != ZCRCG) || len(packet.Payload) > 8192 {
					t.Fatal("unbounded/unacknowledged upload window", packet.End, len(packet.Payload))
				}
				if packet.End == ZCRCQ {
					if output.Len() != 0 {
						t.Fatal("sent next window before ACK")
					}
					feedSend(t, sender, mustHeader(t, ZACK, offset, Hex))
				}
			}
			if !bytes.Equal(received.Bytes(), payload[73:]) {
				t.Fatal("resume payload differs")
			}
			if header := readSendHeader(t, &output, ZEOF); header.Offset() != uint32(len(payload)) {
				t.Fatal(header)
			}
			if file.closed.Load() != 0 {
				t.Fatal("source closed before receiver confirmed EOF")
			}
			feedSend(t, sender, uploadReady(t, format))
			readSendHeader(t, &output, ZFIN)
			feedSend(t, sender, append(mustHeader(t, ZFIN, 0, Hex), []byte("tail")...))
			if output.String() != "OO" || !sender.Done() || final.State != "completed" || final.Transferred != int64(len(payload)) || file.closed.Load() != 1 {
				t.Fatal("upload did not finish", output.String(), final, file.closed.Load())
			}
		})
	}
}
func TestSenderFailureClosesAllSourcesAndSendsAbort(t *testing.T) {
	for _, test := range []struct {
		name     string
		trigger  FrameType
		offset   uint32
		readErr  error
		writeErr bool
	}{
		{name: "resume beyond EOF", trigger: ZRPOS, offset: 20000},
		{name: "read failure", trigger: ZRPOS, readErr: io.ErrUnexpectedEOF},
		{name: "incorrect ACK", trigger: ZACK, offset: 1},
		{name: "write failure", trigger: ZRPOS, writeErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := source(bytes.Repeat([]byte{42}, 9000))
			file.err = test.readErr
			unused := source(nil)
			var output bytes.Buffer
			var final Progress
			failWrites := false
			sender := NewSender(context.Background(), SendOptions{Write: func(data []byte) error {
				if failWrites {
					return io.ErrClosedPipe
				}
				output.Write(data)
				return nil
			}, Select: func(context.Context) ([]Upload, error) {
				return []Upload{{Offer: FileOffer{Name: "first.bin", Size: 9000, SizeKnown: true}, Reader: file}, {Offer: FileOffer{Name: "unused.bin", SizeKnown: true}, Reader: unused}}, nil
			}, Progress: func(value Progress) { final = value }})
			feedSend(t, sender, uploadReady(t, Binary32))
			output.Reset()
			if test.trigger == ZACK {
				feedSend(t, sender, mustHeader(t, ZRPOS, 0, Hex))
				output.Reset()
			}
			failWrites = test.writeErr
			_, err := sender.Feed(mustHeader(t, test.trigger, test.offset, Hex))
			if err == nil || !sender.Done() || final.State != "failed" || file.closed.Load() != 1 || unused.closed.Load() != 1 {
				t.Fatal("failure did not close sources", err, final)
			}
			if !test.writeErr && !bytes.Contains(output.Bytes(), abortSequence) {
				t.Fatal("peer not notified")
			}
			_ = sender.Cancel()
			if file.closed.Load() != 1 {
				t.Fatal("double source close")
			}
		})
	}
}
func TestSenderCancellationInterruptsSelectionAndClosesLateSources(t *testing.T) {
	started := make(chan struct{})
	file := source(nil)
	var final Progress
	sender := NewSender(context.Background(), SendOptions{Write: func([]byte) error { return nil }, Select: func(ctx context.Context) ([]Upload, error) {
		close(started)
		<-ctx.Done()
		return []Upload{{Reader: file}}, nil
	}, Progress: func(value Progress) { final = value }})
	done := make(chan struct{})
	go func() { _, _ = sender.Feed(uploadReady(t, Binary32)); close(done) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("selection did not start")
	}
	_ = sender.Cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt selection")
	}
	if !sender.Done() || !errors.Is(sender.Err(), context.Canceled) || final.State != "cancelled" || final.Error != "" || file.closed.Load() != 1 {
		t.Fatal("late selection leaked", sender.Err(), final, file.closed.Load())
	}
}
func TestSenderRejectsUnsupportedPeerAndMalformedFrames(t *testing.T) {
	for _, wire := range [][]byte{
		mustHeader(t, ZRINIT, 0, Hex),
		mustHeader(t, ZACK, 0, Hex),
		append([]byte{'*', ZDLE, 'C'}, bytes.Repeat([]byte{0}, 9)...),
	} {
		var final Progress
		sender := NewSender(context.Background(), SendOptions{Write: func([]byte) error { return nil }, Progress: func(value Progress) { final = value }})
		_, err := sender.Feed(wire)
		if err == nil || !sender.Done() || final.State != "failed" {
			t.Fatal("invalid peer accepted", err, final)
		}
	}
}
func TestUploadRouterCancelAndSilentTimeoutReleaseInput(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "timeout"}[timeout], func(t *testing.T) {
			started := make(chan struct{})
			events := make(chan Progress, 8)
			duration := time.Second
			if timeout {
				duration = 20 * time.Millisecond
			}
			router := NewRouter(context.Background(), RouterOptions{Timeout: duration, Write: func([]byte) error { return nil }, Select: func(ctx context.Context) ([]Upload, error) {
				close(started)
				<-ctx.Done()
				return nil, context.Cause(ctx)
			}, Progress: func(value Progress) { events <- value }})
			defer router.Close()
			done := make(chan struct{})
			go func() { router.Consume(uploadReady(t, Binary32)); close(done) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("no upload selection")
			}
			if !timeout {
				_ = router.Cancel()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("selection remained blocked")
			}
			last := <-events
			for len(events) > 0 {
				last = <-events
			}
			if timeout {
				if last.State != "failed" || !strings.Contains(last.Error, "timed out") {
					t.Fatal(last)
				}
			} else if last.State != "cancelled" {
				t.Fatal(last)
			}
			// A following ordinary byte is optional; cancellation itself releases input.
			_ = router.Cancel()
			if router.Busy() {
				t.Fatal("input still blocked")
			}
		})
	}
}
