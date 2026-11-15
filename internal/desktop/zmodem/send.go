package zmodem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
)

type SendOptions struct {
	// Abort hands stream ownership back with the framing needed to discard
	// in-flight bytes. It must not reenter the transfer.
	Abort    func(Encoding, bool)
	Write    func([]byte) error
	Select   func(context.Context) ([]Upload, error)
	Progress func(Progress)
}
type sendMode uint8

const (
	sendInit sendMode = iota
	sendAttentionACK
	sendOfferReply
	sendDataACK
	sendFileReady
	sendFinish
)

// Sender streams a bounded data window from retained file descriptors. Each
// window requests an ACK without ending the ZDATA frame (ZCRCQ).
// This keeps slow transports bounded while allowing cancellation and ZRPOS
// recovery between packets. No file data or native path crosses into React.
type Sender struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelCauseFunc
	options  SendOptions
	buffer   []byte
	mode     sendMode
	format   Encoding
	files    []Upload
	index    int
	selected bool
	offset   uint32
	retries  int
	done     bool
	err      error
	progress Progress
}

func NewSender(ctx context.Context, options SendOptions) *Sender {
	child, cancel := context.WithCancelCause(ctx)
	return &Sender{ctx: child, cancel: cancel, options: options, format: Binary16, progress: Progress{State: "running", Direction: "upload", Name: "Waiting for file selection"}}
}
func (s *Sender) Done() bool    { s.mu.Lock(); defer s.mu.Unlock(); return s.done }
func (s *Sender) Err() error    { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *Sender) Cancel() error { return s.Stop(context.Canceled) }
func (s *Sender) Stop(err error) error {
	if err == nil {
		err = context.Canceled
	}
	s.cancel(err)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return s.err
	}
	return s.fail(err, true)
}
func (s *Sender) emit() {
	if s.options.Progress != nil {
		s.options.Progress(s.progress)
	}
}
func (s *Sender) closeFiles() {
	for i := range s.files {
		if s.files[i].Reader != nil {
			_ = s.files[i].Reader.Close()
			s.files[i].Reader = nil
		}
	}
}
func (s *Sender) fail(err error, notify bool) error {
	if s.done {
		return s.err
	}
	err = contextCause(s.ctx, err)
	s.done = true
	s.err = err
	s.cancel(err)
	s.closeFiles()
	s.buffer = nil
	if notify && s.options.Write != nil {
		_ = s.options.Write(append(bytes.Clone(abortSequence), 8, 8, 8, 8, 8))
	}
	if s.options.Abort != nil {
		s.options.Abort(Hex, errors.Is(err, ErrPeerCanceled))
	}
	s.progress.State = "failed"
	s.progress.Error = err.Error()
	if errors.Is(err, context.Canceled) || errors.Is(err, ErrPeerCanceled) {
		s.progress.State = "cancelled"
		s.progress.Error = ""
	}
	s.emit()
	return err
}
func (s *Sender) write(data []byte) error {
	if err := s.ctx.Err(); err != nil {
		return context.Cause(s.ctx)
	}
	if s.options.Write == nil {
		return errors.New("ZMODEM output is unavailable")
	}
	return s.options.Write(data)
}
func (s *Sender) header(header Header, format Encoding) error {
	data, err := EncodeHeader(header, format)
	if err != nil {
		return err
	}
	return s.write(data)
}
func (s *Sender) packet(data []byte, end byte) error {
	encoded, err := EncodeSubpacket(data, end, s.format)
	if err != nil {
		return err
	}
	return s.write(encoded)
}
func (s *Sender) ready(header Header) error {
	flags := header.Data[3]
	if header.Data[0] != 0 || header.Data[1] != 0 {
		return errors.New("bounded receiver buffers are unsupported for ZMODEM upload")
	}
	if flags&3 != 3 || flags&0x80 != 0 {
		return errors.New("ZMODEM upload requires full-duplex overlapping I/O without 8-bit escaping")
	}
	s.format = Binary16
	if flags&0x20 != 0 {
		s.format = Binary32
	}
	if !s.selected {
		s.emit()
		if s.options.Select == nil {
			return errors.New("ZMODEM file selection is unavailable")
		}
		files, err := s.options.Select(s.ctx)
		s.files = files
		s.selected = true
		if err != nil {
			return err
		}
		if err := s.ctx.Err(); err != nil {
			return context.Cause(s.ctx)
		}
		if len(files) == 0 {
			return context.Canceled
		}
		for _, file := range files {
			if file.Reader == nil || !file.Offer.SizeKnown || file.Offer.Size < 0 || file.Offer.Size > math.MaxUint32 {
				return errors.New("invalid ZMODEM upload source")
			}
			parsed, err := parseOffer([]byte(fmt.Sprintf("%s\x00%d\x00", file.Offer.Name, file.Offer.Size)))
			if err != nil || parsed.Name != file.Offer.Name {
				return errors.New("invalid ZMODEM upload filename")
			}
		}
		if flags&0x40 == 0 {
			s.mode = sendAttentionACK
			if err := s.header(Header{Type: ZSINIT, Data: [4]byte{0, 0, 0, 0x40}}, s.format); err != nil {
				return err
			}
			return s.packet([]byte{0}, ZCRCW)
		}
	}
	return s.offerNext()
}
func (s *Sender) offerNext() error {
	if s.index >= len(s.files) {
		s.mode = sendFinish
		return s.header(Header{Type: ZFIN}, Hex)
	}
	file := s.files[s.index]
	var remaining int64
	for _, file := range s.files[s.index:] {
		remaining += file.Offer.Size
	}
	timestamp := file.Offer.Modified.Unix()
	if timestamp < 0 {
		timestamp = 0
	}
	metadata := []byte(fmt.Sprintf("%s\x00%d %o 0 0 %d %d\x00", file.Offer.Name, file.Offer.Size, timestamp, len(s.files)-s.index, remaining))
	if len(metadata) > 16384 {
		return errors.New("ZMODEM upload metadata exceeds its size limit")
	}
	s.offset = 0
	s.retries = 0
	s.mode = sendOfferReply
	if err := s.header(Header{Type: ZFILE}, s.format); err != nil {
		return err
	}
	return s.packet(metadata, ZCRCW)
}

const uploadWindow = 1024 * 1024

func (s *Sender) sendWindow() error {
	for sent := 0; ; {
		file := s.files[s.index]
		remaining := file.Offer.Size - int64(s.offset)
		size := int64(8192)
		if remaining < size {
			size = remaining
		}
		data := make([]byte, int(size))
		if size > 0 {
			count, err := file.Reader.ReadAt(data, int64(s.offset))
			if count != len(data) {
				if err == nil {
					err = io.ErrUnexpectedEOF
				}
				return fmt.Errorf("read ZMODEM upload: %w", err)
			}
			if err != nil && err != io.EOF {
				return err
			}
		}
		end := byte(ZCRCG)
		if remaining == size {
			end = ZCRCE
		} else if sent+int(size) >= uploadWindow {
			end = ZCRCQ
		}
		if err := s.packet(data, end); err != nil {
			return err
		}
		s.offset += uint32(size)
		sent += int(size)
		s.progress.Transferred = int64(s.offset)
		s.emit()
		if end == ZCRCE {
			s.mode = sendFileReady
			return s.header(OffsetHeader(ZEOF, s.offset), Hex)
		}
		if end == ZCRCQ {
			s.mode = sendDataACK
			return nil
		}
	}
}
func (s *Sender) consume(header Header) error {
	if header.Type == ZABORT || header.Type == ZCAN || header.Type == ZFERR {
		return ErrPeerCanceled
	}
	if header.Type == ZCHALLENGE {
		return s.header(OffsetHeader(ZACK, header.Offset()), Hex)
	}
	if header.Type == ZRPOS && (s.mode == sendOfferReply || s.mode == sendDataACK || s.mode == sendFileReady) {
		if header.Offset() > uint32(s.files[s.index].Offer.Size) {
			return errors.New("ZMODEM resume offset exceeds file size")
		}
		s.retries++
		if s.retries > 16 {
			return errors.New("too many ZMODEM resume requests")
		}
		s.offset = header.Offset()
		file := s.files[s.index]
		s.progress = Progress{State: "running", Direction: "upload", Name: file.Offer.Name, Total: file.Offer.Size, Transferred: int64(s.offset)}
		s.emit()
		if err := s.header(OffsetHeader(ZDATA, s.offset), s.format); err != nil {
			return err
		}
		return s.sendWindow()
	}
	switch s.mode {
	case sendInit:
		if header.Type == ZRINIT {
			return s.ready(header)
		}
	case sendAttentionACK:
		if header.Type == ZACK {
			return s.offerNext()
		}
	case sendOfferReply:
		if header.Type == ZSKIP {
			_ = s.files[s.index].Reader.Close()
			s.files[s.index].Reader = nil
			s.index++
			return s.offerNext()
		}
		if header.Type == ZRINIT {
			return nil
		} // receiver keepalive while native panel was open
	case sendDataACK:
		if header.Type == ZACK {
			if header.Offset() != s.offset {
				return errors.New("ZMODEM upload acknowledgement offset mismatch")
			}
			return s.sendWindow()
		}
	case sendFileReady:
		if header.Type == ZRINIT {
			_ = s.files[s.index].Reader.Close()
			s.files[s.index].Reader = nil
			s.index++
			return s.ready(header)
		}
	case sendFinish:
		if header.Type == ZFIN {
			if err := s.write([]byte("OO")); err != nil {
				return err
			}
			s.done = true
			s.cancel(nil)
			s.closeFiles()
			s.progress.State = "completed"
			s.emit()
			return nil
		}
	}
	return fmt.Errorf("unexpected ZMODEM sender header %d in state %d", header.Type, s.mode)
}
func (s *Sender) Feed(input []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return bytes.Clone(input), s.err
	}
	if err := s.ctx.Err(); err != nil {
		return nil, s.fail(context.Cause(s.ctx), true)
	}
	if len(s.buffer)+len(input) > 65536 {
		return nil, s.fail(ErrTooLarge, true)
	}
	s.buffer = append(s.buffer, input...)
	if index := bytes.Index(s.buffer, abortSequence); index >= 0 {
		tail := bytes.Clone(s.buffer[index+len(abortSequence):])
		for len(tail) > 0 && tail[0] == 8 {
			tail = tail[1:]
		}
		return tail, s.fail(ErrPeerCanceled, false)
	}
	for len(s.buffer) > 0 {
		if err := s.ctx.Err(); err != nil {
			return nil, s.fail(context.Cause(s.ctx), true)
		}
		// Discard protocol diagnostics/flow-control before the next header.
		if s.buffer[0] != '*' {
			s.buffer = s.buffer[1:]
			continue
		}
		header, size, err := DecodeHeader(s.buffer)
		if errors.Is(err, ErrIncomplete) {
			return nil, nil
		}
		if err != nil {
			return nil, s.fail(err, true)
		}
		s.buffer = s.buffer[size:]
		if err = s.consume(header); err != nil {
			return nil, s.fail(err, true)
		}
		if s.done {
			tail := bytes.Clone(s.buffer)
			s.buffer = nil
			return tail, nil
		}
	}
	return nil, nil
}
