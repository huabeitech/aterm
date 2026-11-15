// Package zmodem implements the terminal transfer protocol independently of
// the renderer. Wire parsing never mutates its input on incomplete frames.
package zmodem

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
)

type FrameType byte

const (
	ZRQINIT FrameType = iota
	ZRINIT
	ZSINIT
	ZACK
	ZFILE
	ZSKIP
	ZNAK
	ZABORT
	ZFIN
	ZRPOS
	ZDATA
	ZEOF
	ZFERR
	ZCRC
	ZCHALLENGE
	ZCOMPL
	ZCAN
	ZFREECNT
	ZCOMMAND
	ZSTDERR
)

type Encoding byte

const (
	Binary16 Encoding = 'A'
	Hex      Encoding = 'B'
	Binary32 Encoding = 'C'
)
const (
	ZDLE         byte = 0x18
	ZCRCE        byte = 'h'
	ZCRCG        byte = 'i'
	ZCRCQ        byte = 'j'
	ZCRCW        byte = 'k'
	MaxSubpacket      = 1024 * 1024
)

var (
	ErrIncomplete = errors.New("incomplete ZMODEM frame")
	ErrMalformed  = errors.New("invalid ZMODEM frame")
	ErrCRC        = errors.New("ZMODEM checksum mismatch")
	ErrTooLarge   = errors.New("ZMODEM subpacket exceeds its size limit")
)

type Header struct {
	Type     FrameType
	Data     [4]byte
	Encoding Encoding
}

func OffsetHeader(kind FrameType, offset uint32) Header {
	header := Header{Type: kind}
	binary.LittleEndian.PutUint32(header.Data[:], offset)
	return header
}
func (h Header) Offset() uint32 { return binary.LittleEndian.Uint32(h.Data[:]) }

type Subpacket struct {
	Payload []byte
	End     byte
}

func (p Subpacket) EndsFrame() bool { return p.End == ZCRCE || p.End == ZCRCW }
func (p Subpacket) NeedsACK() bool  { return p.End == ZCRCQ || p.End == ZCRCW }

// CRC-CCITT/XMODEM, initial zero, no final XOR. Header CRC16 is big endian;
// CRC32 uses IEEE with the protocol's little-endian representation.
func checksum(data []byte, format Encoding) []byte {
	if format == Binary32 {
		result := make([]byte, 4)
		binary.LittleEndian.PutUint32(result, crc32.ChecksumIEEE(data))
		return result
	}
	var value uint16
	for _, octet := range data {
		value ^= uint16(octet) << 8
		for bit := 0; bit < 8; bit++ {
			if value&0x8000 != 0 {
				value = (value << 1) ^ 0x1021
			} else {
				value <<= 1
			}
		}
	}
	return []byte{byte(value >> 8), byte(value)}
}
func validEncoding(format Encoding) bool {
	return format == Hex || format == Binary16 || format == Binary32
}
func crcLength(format Encoding) int {
	if format == Binary32 {
		return 4
	}
	return 2
}
func escape(data []byte) []byte {
	result := make([]byte, 0, len(data)*2)
	for _, octet := range data {
		if octet&0x60 == 0 {
			result = append(result, ZDLE, octet^0x40)
		} else {
			result = append(result, octet)
		}
	}
	return result
}
func flowControl(octet byte) bool {
	return octet == 0x11 || octet == 0x13 || octet == 0x91 || octet == 0x93
}

// Decode one octet, ignoring unescaped link-level XON/XOFF. Frame terminators
// are deliberately rejected here and handled only by the subpacket reader.
func decodeOctet(input []byte) (byte, int, error) {
	index := 0
	for index < len(input) && flowControl(input[index]) {
		index++
	}
	if index == len(input) {
		return 0, 0, ErrIncomplete
	}
	octet := input[index]
	index++
	if octet != ZDLE {
		return octet, index, nil
	}
	for index < len(input) && flowControl(input[index]) {
		index++
	}
	if index == len(input) {
		return 0, 0, ErrIncomplete
	}
	escaped := input[index]
	index++
	switch escaped {
	case 'l':
		return 0x7f, index, nil
	case 'm':
		return 0xff, index, nil
	}
	if escaped&0x60 != 0x40 {
		return 0, 0, ErrMalformed
	}
	return escaped ^ 0x40, index, nil
}
func decodeOctets(input []byte, count int) ([]byte, int, error) {
	result := make([]byte, 0, count)
	consumed := 0
	for len(result) < count {
		octet, size, err := decodeOctet(input[consumed:])
		if err != nil {
			return nil, 0, err
		}
		consumed += size
		result = append(result, octet)
	}
	return result, consumed, nil
}
func EncodeHeader(header Header, format Encoding) ([]byte, error) {
	if !validEncoding(format) {
		return nil, ErrMalformed
	}
	payload := append([]byte{byte(header.Type)}, header.Data[:]...)
	data := append(payload, checksum(payload, format)...)
	if format == Hex {
		result := []byte{'*', '*', ZDLE, byte(Hex)}
		result = append(result, []byte(hex.EncodeToString(data))...)
		result = append(result, '\r', '\n')
		if header.Type != ZACK && header.Type != ZFIN {
			result = append(result, 0x11)
		}
		return result, nil
	}
	return append([]byte{'*', ZDLE, byte(format)}, escape(data)...), nil
}
func DecodeHeader(input []byte) (Header, int, error) {
	if len(input) < 1 {
		return Header{}, 0, ErrIncomplete
	}
	if input[0] != '*' {
		return Header{}, 0, ErrMalformed
	}
	if len(input) < 2 {
		return Header{}, 0, ErrIncomplete
	}
	prefix := 3
	var format Encoding
	if input[1] == '*' {
		prefix = 4
		if len(input) < 3 {
			return Header{}, 0, ErrIncomplete
		}
		if input[2] != ZDLE {
			return Header{}, 0, ErrMalformed
		}
		if len(input) < 4 {
			return Header{}, 0, ErrIncomplete
		}
		format = Encoding(input[3])
		if format != Hex {
			return Header{}, 0, ErrMalformed
		}
	} else {
		if input[1] != ZDLE {
			return Header{}, 0, ErrMalformed
		}
		if len(input) < 3 {
			return Header{}, 0, ErrIncomplete
		}
		format = Encoding(input[2])
		if format != Binary16 && format != Binary32 {
			return Header{}, 0, ErrMalformed
		}
	}
	var data []byte
	consumed := prefix
	if format == Hex {
		if len(input) < prefix+14 {
			return Header{}, 0, ErrIncomplete
		}
		data = make([]byte, 7)
		if _, err := hex.Decode(data, input[prefix:prefix+14]); err != nil {
			return Header{}, 0, ErrMalformed
		}
		consumed += 14
		if len(input) == consumed {
			return Header{}, 0, ErrIncomplete
		}
		if input[consumed] == 0x0d || input[consumed] == 0x8d {
			consumed++
			if len(input) == consumed {
				return Header{}, 0, ErrIncomplete
			}
		}
		if input[consumed] != 0x0a && input[consumed] != 0x8a {
			return Header{}, 0, ErrMalformed
		}
		consumed++
		if len(input) > consumed && (input[consumed] == 0x11 || input[consumed] == 0x91) {
			consumed++
		}
	} else {
		decoded, size, err := decodeOctets(input[prefix:], 5+crcLength(format))
		if err != nil {
			return Header{}, 0, err
		}
		data = decoded
		consumed += size
	}
	if !bytes.Equal(checksum(data[:5], format), data[5:]) {
		return Header{}, 0, ErrCRC
	}
	header := Header{Type: FrameType(data[0]), Encoding: format}
	copy(header.Data[:], data[1:5])
	return header, consumed, nil
}
func validEnd(end byte) bool { return end >= ZCRCE && end <= ZCRCW }
func EncodeSubpacket(payload []byte, end byte, format Encoding) ([]byte, error) {
	if !validEncoding(format) || !validEnd(end) {
		return nil, ErrMalformed
	}
	if len(payload) > MaxSubpacket {
		return nil, ErrTooLarge
	}
	result := escape(payload)
	result = append(result, ZDLE, end)
	checked := append(append(make([]byte, 0, len(payload)+1), payload...), end)
	return append(result, escape(checksum(checked, format))...), nil
}
func DecodeSubpacket(input []byte, format Encoding) (Subpacket, int, error) {
	if !validEncoding(format) {
		return Subpacket{}, 0, ErrMalformed
	}
	payload := make([]byte, 0, min(len(input), MaxSubpacket))
	consumed := 0
	for {
		// Ignore flow control before looking for a terminator, including a split
		// ZDLE/terminator pair across terminal reads.
		for consumed < len(input) && flowControl(input[consumed]) {
			consumed++
		}
		if consumed == len(input) {
			return Subpacket{}, 0, ErrIncomplete
		}
		if input[consumed] == ZDLE {
			after := consumed + 1
			for after < len(input) && flowControl(input[after]) {
				after++
			}
			if after == len(input) {
				return Subpacket{}, 0, ErrIncomplete
			}
			if validEnd(input[after]) {
				end := input[after]
				crc, size, err := decodeOctets(input[after+1:], crcLength(format))
				if err != nil {
					return Subpacket{}, 0, err
				}
				checked := append(append(make([]byte, 0, len(payload)+1), payload...), end)
				if !bytes.Equal(crc, checksum(checked, format)) {
					return Subpacket{}, 0, ErrCRC
				}
				return Subpacket{Payload: payload, End: end}, after + 1 + size, nil
			}
		}
		octet, size, err := decodeOctet(input[consumed:])
		if err != nil {
			return Subpacket{}, 0, err
		}
		consumed += size
		payload = append(payload, octet)
		if len(payload) > MaxSubpacket {
			return Subpacket{}, 0, ErrTooLarge
		}
	}
}
