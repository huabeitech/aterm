package zmodem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

type stagedDownload struct {
	bytes.Buffer
	committed, aborted int
	partial            int
	commitError        error
	zeroWrite          bool
}

func (d *stagedDownload) Write(data []byte) (int, error) {
	if d.zeroWrite {
		return 0, nil
	}
	if d.partial > 0 && len(data) > d.partial {
		data = data[:d.partial]
	}
	return d.Buffer.Write(data)
}
func (d *stagedDownload) Commit() error {
	if d.commitError != nil {
		return d.commitError
	}
	d.committed++
	return nil
}
func (d *stagedDownload) Abort() error { d.aborted++; return nil }
func mustHeader(t *testing.T, kind FrameType, offset uint32, format Encoding) []byte {
	t.Helper()
	data, err := EncodeHeader(OffsetHeader(kind, offset), format)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func mustPacket(t *testing.T, data []byte, end byte, format Encoding) []byte {
	t.Helper()
	wire, err := EncodeSubpacket(data, end, format)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}
func feedFragments(t *testing.T, r *Receiver, data []byte) {
	t.Helper()
	for len(data) > 0 {
		size := min(len(data), 7)
		if _, err := r.Feed(data[:size]); err != nil {
			t.Fatal(err)
		}
		data = data[size:]
	}
}
func offerWire(t *testing.T, size int, format Encoding) []byte {
	data := mustHeader(t, ZFILE, 1<<24, format)
	return append(data, mustPacket(t, []byte(fmt.Sprintf("nested/space 中文.bin\x00%d 0 0 0 1 %d\x00", size, size)), ZCRCW, format)...)
}
func TestReceiveSessionStreamsCommitsAndRestoresTerminalOutput(t *testing.T) {
	for _, format := range []Encoding{Binary16, Binary32} {
		t.Run(fmt.Sprintf("crc%d", crcLength(format)*8), func(t *testing.T) {
			file := &stagedDownload{partial: 3}
			var outgoing bytes.Buffer
			var progress []Progress
			offers := 0
			receiver := NewReceiver(context.Background(), ReceiveOptions{Write: func(data []byte) error { _, err := outgoing.Write(data); return err }, Offer: func(_ context.Context, offer FileOffer) (Download, error) {
				offers++
				if offer.Name != "space 中文.bin" || offer.Size != 1024 || !offer.SizeKnown {
					t.Error("file metadata lost", offer)
				}
				return file, nil
			}, Progress: func(value Progress) { progress = append(progress, value) }})
			feedFragments(t, receiver, mustHeader(t, ZRQINIT, 0, Hex))
			// Sender attention and escaping negotiation precede an offer.
			feedFragments(t, receiver, append(mustHeader(t, ZSINIT, 0, format), mustPacket(t, []byte{0}, ZCRCW, format)...))
			feedFragments(t, receiver, offerWire(t, 1024, format))
			payload := make([]byte, 1024)
			for i := range payload {
				payload[i] = byte(i)
			}
			feedFragments(t, receiver, mustHeader(t, ZDATA, 0, format))
			feedFragments(t, receiver, mustPacket(t, payload[:256], ZCRCQ, format))
			feedFragments(t, receiver, mustPacket(t, payload[256:512], ZCRCW, format))
			feedFragments(t, receiver, mustHeader(t, ZDATA, 512, format))
			feedFragments(t, receiver, mustPacket(t, payload[512:], ZCRCE, format))
			if file.committed != 0 {
				t.Fatal("file published before ZEOF")
			}
			feedFragments(t, receiver, mustHeader(t, ZEOF, 1024, Hex))
			if file.committed != 1 || file.aborted != 0 || !bytes.Equal(file.Bytes(), payload) || offers != 1 {
				t.Fatal("received file was not committed exactly once")
			}
			feedFragments(t, receiver, append([]byte("skip diagnostic *word*\r\n"), mustHeader(t, ZFIN, 0, Hex)...))
			if receiver.Done() {
				t.Fatal("session finished before OO")
			}
			tail, err := receiver.Feed([]byte("OOshell> "))
			if err != nil || string(tail) != "shell> " || !receiver.Done() {
				t.Fatal("terminal output not restored", string(tail), err)
			}
			if last := progress[len(progress)-1]; last.State != "completed" || last.Transferred != 1024 || last.Total != 1024 {
				t.Fatal("incorrect final progress", last)
			}
			var types []FrameType
			wire := outgoing.Bytes()
			for len(wire) > 0 {
				header, used, err := DecodeHeader(wire)
				if err != nil {
					t.Fatal(err)
				}
				types = append(types, header.Type)
				wire = wire[used:]
			}
			expected := []FrameType{ZRINIT, ZACK, ZRPOS, ZACK, ZACK, ZRINIT, ZFIN}
			if fmt.Sprint(types) != fmt.Sprint(expected) {
				t.Fatal("incorrect receive handshake", types)
			}
		})
	}
}
func receivingFixture(t *testing.T, file *stagedDownload) (*Receiver, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	receiver := NewReceiver(context.Background(), ReceiveOptions{Write: func(data []byte) error { _, err := output.Write(data); return err }, Offer: func(context.Context, FileOffer) (Download, error) { return file, nil }})
	feedFragments(t, receiver, mustHeader(t, ZRQINIT, 0, Hex))
	feedFragments(t, receiver, offerWire(t, 4, Binary32))
	return receiver, output
}
func TestReceiveRejectsCorruptionWrongOffsetsAndIncompleteFiles(t *testing.T) {
	for _, kind := range []string{"crc", "offset", "size", "publish", "write"} {
		t.Run(kind, func(t *testing.T) {
			file := &stagedDownload{}
			receiver, output := receivingFixture(t, file)
			var input []byte
			switch kind {
			case "offset":
				input = mustHeader(t, ZDATA, 1, Binary32)
			default:
				feedFragments(t, receiver, mustHeader(t, ZDATA, 0, Binary32))
				packet := mustPacket(t, []byte("data"), ZCRCE, Binary32)
				if kind == "crc" {
					packet[0] ^= 1
				}
				if kind == "write" {
					file.zeroWrite = true
				}
				if kind == "publish" {
					file.commitError = errors.New("fixture publication failed")
				}
				input = packet
				if kind == "size" || kind == "publish" {
					feedFragments(t, receiver, packet)
					offset := uint32(4)
					if kind == "size" {
						offset = 3
					}
					input = mustHeader(t, ZEOF, offset, Hex)
				}
			}
			_, err := receiver.Feed(input)
			if err == nil {
				t.Fatal("invalid transfer accepted")
			}
			if kind == "write" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatal("non-progressing writer did not fail", err)
			}
			if !receiver.Done() || file.committed != 0 || file.aborted != 1 || !bytes.HasSuffix(output.Bytes(), []byte{ZDLE, ZDLE, ZDLE, ZDLE, ZDLE, 8, 8, 8, 8, 8}) {
				t.Fatal("failed transfer did not clean staged data")
			}
			receiver.Cancel()
			if file.aborted != 1 {
				t.Fatal("cancel repeated cleanup")
			}
		})
	}
}
func TestReceiveCancelInterruptsPendingFileSelection(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	receiver := NewReceiver(context.Background(), ReceiveOptions{Write: func([]byte) error { return nil }, Offer: func(ctx context.Context, _ FileOffer) (Download, error) {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	done := make(chan error, 1)
	input := offerWire(t, 4, Binary32)
	go func() { _, err := receiver.Feed(input); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("file selection did not start")
	}
	if err := receiver.Cancel(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("file selection ignored cancellation")
	}
}
func TestReceivePeerCancelSkipAndInvalidMetadata(t *testing.T) {
	file := &stagedDownload{}
	receiver, _ := receivingFixture(t, file)
	tail, err := receiver.Feed(append(bytes.Clone(abortSequence), 8, 8, 8, 8, 8, '$', ' '))
	if !errors.Is(err, ErrPeerCanceled) || string(tail) != "$ " || file.aborted != 1 {
		t.Fatal("peer cancellation did not preserve terminal tail", err, string(tail))
	}
	var output bytes.Buffer
	skipped := NewReceiver(context.Background(), ReceiveOptions{Write: func(data []byte) error { _, err := output.Write(data); return err }, Offer: func(context.Context, FileOffer) (Download, error) { return nil, ErrSkip }})
	feedFragments(t, skipped, offerWire(t, 4, Binary16))
	header, _, err := DecodeHeader(output.Bytes())
	if err != nil || header.Type != ZSKIP {
		t.Fatal("declined file was not skipped", err)
	}
	for _, payload := range [][]byte{[]byte("missing separator"), []byte("..\x004"), []byte("/\x004"), []byte("bad\nname\x004"), []byte("file\x004294967296"), []byte("file\x00-1")} {
		if _, err := parseOffer(payload); err == nil {
			t.Fatal("invalid metadata accepted", payload)
		}
	}
}

func TestReceiveBatchResetsOffsetsAndSupportsEmptyFiles(t *testing.T) {
	files := []*stagedDownload{}
	receiver := NewReceiver(context.Background(), ReceiveOptions{Write: func([]byte) error { return nil }, Offer: func(context.Context, FileOffer) (Download, error) {
		file := &stagedDownload{}
		files = append(files, file)
		return file, nil
	}})
	feedFragments(t, receiver, mustHeader(t, ZRQINIT, 0, Hex))
	for _, payload := range [][]byte{[]byte("first"), nil, []byte("last")} {
		feedFragments(t, receiver, offerWire(t, len(payload), Binary32))
		feedFragments(t, receiver, mustHeader(t, ZDATA, 0, Binary32))
		feedFragments(t, receiver, mustPacket(t, payload, ZCRCE, Binary32))
		feedFragments(t, receiver, mustHeader(t, ZEOF, uint32(len(payload)), Hex))
	}
	feedFragments(t, receiver, mustHeader(t, ZFIN, 0, Hex))
	receiver.Feed([]byte("OO"))
	if len(files) != 3 || !receiver.Done() || receiver.Err() != nil {
		t.Fatal("batch did not complete")
	}
	if files[0].String() != "first" || files[1].Len() != 0 || files[2].String() != "last" {
		t.Fatal("batch file data or offsets leaked between offers")
	}
	for _, file := range files {
		if file.committed != 1 || file.aborted != 0 {
			t.Fatal("batch file not committed exactly once")
		}
	}
}
