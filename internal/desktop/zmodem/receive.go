package zmodem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var ErrSkip = errors.New("ZMODEM file skipped")
var ErrPeerCanceled = errors.New("ZMODEM transfer canceled by peer")
var abortSequence = []byte{ZDLE, ZDLE, ZDLE, ZDLE, ZDLE}

type FileOffer struct {
	Name      string
	Size      int64
	SizeKnown bool
	Modified  time.Time
}

// Download owns a staged file. Commit publishes it only after a valid ZEOF;
// Abort must discard any partial data and is never called after Commit succeeds.
type Download interface {
	io.Writer
	Commit() error
	Abort() error
}
type Progress struct {
	State       string `json:"state"`
	Direction   string `json:"direction"`
	Name        string `json:"name"`
	Transferred int64  `json:"transferred"`
	Total       int64  `json:"total"`
	Error       string `json:"error,omitempty"`
}
type ReceiveOptions struct {
	// Abort hands stream ownership back with the framing needed to discard
	// in-flight bytes. It must not reenter the transfer.
	Abort    func(Encoding, bool)
	Write    func([]byte) error
	Offer    func(context.Context, FileOffer) (Download, error)
	Progress func(Progress)
}
type receiveMode uint8

const (
	receiveHeader receiveMode = iota
	receiveInitData
	receiveOfferData
	receiveFileData
	receiveOverAndOut
)

// Receiver consumes a detected receiving session, including its initial
// ZRQINIT. Feed calls are serialized; callbacks must not reenter the receiver.
// Cancel interrupts the offer context before waiting for in-flight Feed work.
type Receiver struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelCauseFunc
	options  ReceiveOptions
	buffer   []byte
	mode     receiveMode
	format   Encoding
	file     Download
	offer    FileOffer
	offset   uint32
	progress Progress
	done     bool
	err      error
}

func NewReceiver(ctx context.Context, options ReceiveOptions) *Receiver {
	child, cancel := context.WithCancelCause(ctx)
	return &Receiver{ctx: child, cancel: cancel, options: options, progress: Progress{State: "running", Direction: "download", Name: "Waiting for file selection"}}
}
func (r *Receiver) Done() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.done }
func (r *Receiver) Err() error { r.mu.Lock(); defer r.mu.Unlock(); return r.err }
func (r *Receiver) emit() {
	if r.options.Progress != nil {
		r.options.Progress(r.progress)
	}
}
func (r *Receiver) send(header Header) error {
	if r.options.Write == nil {
		return errors.New("ZMODEM output is unavailable")
	}
	data, err := EncodeHeader(header, Hex)
	if err != nil {
		return err
	}
	return r.options.Write(data)
}
func (r *Receiver) ready() error { return r.send(Header{Type: ZRINIT, Data: [4]byte{0, 0, 0, 0x23}}) }
func (r *Receiver) fail(err error, notify bool) error {
	if r.done {
		return r.err
	}
	if cause := context.Cause(r.ctx); cause != nil {
		err = cause
	}
	r.done = true
	r.err = err
	r.cancel(err)
	if r.file != nil {
		_ = r.file.Abort()
		r.file = nil
	}
	r.buffer = nil
	if notify && r.options.Write != nil {
		_ = r.options.Write(append(bytes.Clone(abortSequence), 8, 8, 8, 8, 8))
	}
	if r.options.Abort != nil {
		r.options.Abort(r.format, errors.Is(err, ErrPeerCanceled))
	}
	r.progress.State = "failed"
	r.progress.Error = err.Error()
	if errors.Is(err, context.Canceled) || errors.Is(err, ErrPeerCanceled) {
		r.progress.State = "cancelled"
		r.progress.Error = ""
	}
	r.progress.Transferred = int64(r.offset)
	r.emit()
	return err
}
func (r *Receiver) Cancel() error { return r.Stop(context.Canceled) }
func (r *Receiver) Stop(err error) error {
	if err == nil {
		err = context.Canceled
	}
	r.cancel(err)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return r.err
	}
	return r.fail(contextCause(r.ctx, err), true)
}
func parseOffer(payload []byte) (FileOffer, error) {
	if len(payload) > 16*1024 {
		return FileOffer{}, errors.New("ZMODEM file metadata exceeds its size limit")
	}
	split := bytes.IndexByte(payload, 0)
	if split < 1 {
		return FileOffer{}, errors.New("invalid ZMODEM filename")
	}
	raw := string(payload[:split])
	if !utf8.ValidString(raw) {
		return FileOffer{}, errors.New("invalid UTF-8 ZMODEM filename")
	}
	name := path.Base(strings.ReplaceAll(raw, "\\", "/"))
	if name == "." || name == ".." || strings.ContainsAny(name, "/\\") || name == "" {
		return FileOffer{}, errors.New("invalid ZMODEM filename")
	}
	for _, value := range name {
		if value < 32 {
			return FileOffer{}, errors.New("invalid ZMODEM filename")
		}
	}
	offer := FileOffer{Name: name}
	fields := strings.Fields(strings.TrimRight(string(payload[split+1:]), "\x00"))
	if len(fields) > 0 {
		size, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			return FileOffer{}, errors.New("invalid ZMODEM file size")
		}
		offer.Size = int64(size)
		offer.SizeKnown = true
	}
	if len(fields) > 1 {
		if timestamp, err := strconv.ParseInt(fields[1], 8, 64); err == nil && timestamp > 0 {
			offer.Modified = time.Unix(timestamp, 0)
		}
	}
	return offer, nil
}

// Feed returns bytes following the final OO, which belong to ordinary terminal
// output. ErrIncomplete is internal: callers retain neither fragments nor CRCs.
func (r *Receiver) Feed(input []byte) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return bytes.Clone(input), r.err
	}
	if err := r.ctx.Err(); err != nil {
		return nil, r.fail(contextCause(r.ctx, err), true)
	}
	if len(r.buffer)+len(input) > 2*MaxSubpacket+128 {
		return nil, r.fail(ErrTooLarge, true)
	}
	r.buffer = append(r.buffer, input...)
	if index := bytes.Index(r.buffer, abortSequence); index >= 0 {
		tail := bytes.Clone(r.buffer[index+len(abortSequence):])
		for len(tail) > 0 && tail[0] == 8 {
			tail = tail[1:]
		}
		return tail, r.fail(ErrPeerCanceled, false)
	}
	for len(r.buffer) > 0 {
		if err := r.ctx.Err(); err != nil {
			return nil, r.fail(contextCause(r.ctx, err), true)
		}
		if r.mode == receiveOverAndOut {
			if len(r.buffer) < 2 {
				return nil, nil
			}
			if r.buffer[0] != 'O' || r.buffer[1] != 'O' {
				return nil, r.fail(errors.New("invalid ZMODEM session ending"), true)
			}
			tail := bytes.Clone(r.buffer[2:])
			r.buffer = nil
			r.done = true
			r.cancel(nil)
			r.progress.State = "completed"
			r.emit()
			return tail, nil
		}
		if r.mode != receiveHeader {
			packet, size, err := DecodeSubpacket(r.buffer, r.format)
			if errors.Is(err, ErrIncomplete) {
				return nil, nil
			}
			if err != nil {
				return nil, r.fail(contextCause(r.ctx, err), true)
			}
			r.buffer = r.buffer[size:]
			if err = r.consumePacket(packet); err != nil {
				return nil, r.fail(contextCause(r.ctx, err), true)
			}
			continue
		}
		// lrzsz may emit CR/LF or file-skip diagnostics between headers. They are
		// protocol noise while a session is active, not user terminal output.
		if !r.findHeaderPrefix() {
			return nil, nil
		}
		header, size, err := DecodeHeader(r.buffer)
		if errors.Is(err, ErrIncomplete) {
			return nil, nil
		}
		if err != nil {
			return nil, r.fail(contextCause(r.ctx, err), true)
		}
		r.buffer = r.buffer[size:]
		r.format = header.Encoding
		if err = r.consumeHeader(header); err != nil {
			return nil, r.fail(contextCause(r.ctx, err), true)
		}
	}
	return nil, nil
}
func (r *Receiver) consumeHeader(header Header) error {
	switch header.Type {
	case ZRQINIT:
		if r.file != nil {
			return ErrMalformed
		}
		r.emit()
		return r.ready()
	case ZSINIT:
		if r.file != nil {
			return ErrMalformed
		}
		r.mode = receiveInitData
	case ZFILE:
		if r.file != nil {
			return errors.New("a ZMODEM file is already active")
		}
		if header.Data[1] != 0 || header.Data[0]&0x40 != 0 {
			return errors.New("compressed or sparse ZMODEM files are unsupported")
		}
		r.mode = receiveOfferData
	case ZDATA:
		if r.file == nil || header.Offset() != r.offset {
			return errors.New("ZMODEM data offset mismatch")
		}
		r.mode = receiveFileData
	case ZEOF:
		if r.file == nil || header.Offset() != r.offset || (r.offer.SizeKnown && int64(r.offset) != r.offer.Size) {
			return errors.New("ZMODEM end-of-file size mismatch")
		}
		if err := r.file.Commit(); err != nil {
			return fmt.Errorf("publish ZMODEM download: %w", err)
		}
		r.file = nil
		r.emit()
		return r.ready()
	case ZFIN:
		if r.file != nil {
			return errors.New("ZMODEM session ended before its file completed")
		}
		r.mode = receiveOverAndOut
		return r.send(Header{Type: ZFIN})
	case ZABORT, ZCAN:
		return ErrPeerCanceled
	default:
		return fmt.Errorf("unexpected ZMODEM receiver header %d", header.Type)
	}
	return nil
}
func (r *Receiver) consumePacket(packet Subpacket) error {
	switch r.mode {
	case receiveInitData:
		if !packet.EndsFrame() {
			return ErrMalformed
		}
		r.mode = receiveHeader
		return r.send(Header{Type: ZACK})
	case receiveOfferData:
		if !packet.EndsFrame() {
			return ErrMalformed
		}
		offer, err := parseOffer(packet.Payload)
		if err != nil {
			return err
		}
		r.offer = offer
		r.offset = 0
		r.progress = Progress{State: "running", Direction: "download", Name: offer.Name, Total: offer.Size}
		r.emit()
		if r.options.Offer == nil {
			return errors.New("ZMODEM file selection is unavailable")
		}
		file, err := r.options.Offer(r.ctx, offer)
		if errors.Is(err, ErrSkip) {
			if file != nil {
				file.Abort()
			}
			r.mode = receiveHeader
			return r.send(Header{Type: ZSKIP})
		}
		if err != nil {
			if file != nil {
				file.Abort()
			}
			return err
		}
		if file == nil {
			return errors.New("ZMODEM download was not opened")
		}
		r.file = file
		if err = r.ctx.Err(); err != nil {
			return err
		}
		r.mode = receiveHeader
		return r.send(OffsetHeader(ZRPOS, 0))
	case receiveFileData:
		if r.file == nil {
			return errors.New("ZMODEM data received without an accepted file")
		}
		if uint64(r.offset)+uint64(len(packet.Payload)) > math.MaxUint32 || (r.offer.SizeKnown && int64(r.offset)+int64(len(packet.Payload)) > r.offer.Size) {
			return errors.New("ZMODEM data exceeds declared file size")
		}
		remaining := packet.Payload
		for len(remaining) > 0 {
			count, err := r.file.Write(remaining)
			if count < 0 || count > len(remaining) {
				return io.ErrShortWrite
			}
			r.offset += uint32(count)
			remaining = remaining[count:]
			if err != nil {
				return err
			}
			if count == 0 {
				return io.ErrShortWrite
			}
		}
		r.progress.Transferred = int64(r.offset)
		r.emit()
		if packet.EndsFrame() {
			r.mode = receiveHeader
		}
		if packet.NeedsACK() {
			return r.send(OffsetHeader(ZACK, r.offset))
		}
	}
	return nil
}

// Retain only a possible split prefix while ignoring between-frame diagnostics.
func (r *Receiver) findHeaderPrefix() bool {
	for len(r.buffer) > 0 {
		if r.buffer[0] != '*' {
			r.buffer = r.buffer[1:]
			continue
		}
		if len(r.buffer) < 2 {
			return false
		}
		if r.buffer[1] == '*' {
			if len(r.buffer) < 3 {
				return false
			}
			if r.buffer[2] != ZDLE {
				r.buffer = r.buffer[1:]
				continue
			}
			if len(r.buffer) < 4 {
				return false
			}
			if r.buffer[3] == byte(Hex) {
				return true
			}
		} else if r.buffer[1] == ZDLE {
			if len(r.buffer) < 3 {
				return false
			}
			if r.buffer[2] == byte(Binary16) || r.buffer[2] == byte(Binary32) {
				return true
			}
		}
		r.buffer = r.buffer[1:]
	}
	return false
}

func contextCause(ctx context.Context, fallback error) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return fallback
}
