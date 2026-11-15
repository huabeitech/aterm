package zmodem

import (
	"bytes"
	"errors"
	"testing"
)

func TestStandardChecksumVectors(t *testing.T) {
	if got := checksum([]byte("123456789"), Binary16); !bytes.Equal(got, []byte{0x31, 0xc3}) {
		t.Fatalf("CRC16 %x", got)
	}
	if got := checksum([]byte("123456789"), Binary32); !bytes.Equal(got, []byte{0x26, 0x39, 0xf4, 0xcb}) {
		t.Fatalf("CRC32 %x", got)
	}
}
func TestHeaderRoundTripAndFragmentation(t *testing.T) {
	for _, format := range []Encoding{Hex, Binary16, Binary32} {
		for _, kind := range []FrameType{ZRQINIT, ZRINIT, ZACK, ZFIN, ZRPOS, ZDATA, ZEOF} {
			expected := OffsetHeader(kind, 0xff181100)
			encoded, err := EncodeHeader(expected, format)
			if err != nil {
				t.Fatal(err)
			}
			before := bytes.Clone(encoded)
			actual, size, err := DecodeHeader(encoded)
			if err != nil || size != len(encoded) || actual.Type != kind || actual.Data != expected.Data || actual.Encoding != format {
				t.Fatalf("header %v/%v: %+v %d %v", kind, format, actual, size, err)
			}
			if !bytes.Equal(before, encoded) {
				t.Fatal("decoder mutated input")
			}
			required := len(encoded)
			if format == Hex && encoded[len(encoded)-1] == 0x11 {
				required--
			}
			for n := 0; n < required; n++ {
				if _, used, err := DecodeHeader(encoded[:n]); !errors.Is(err, ErrIncomplete) || used != 0 {
					t.Fatalf("fragment %v/%v length %d: used=%d err=%v", kind, format, n, used, err)
				}
			}
		}
	}
}
func TestSubpacketAllOctetsAndTerminators(t *testing.T) {
	payload := make([]byte, 256)
	for i := range payload {
		payload[i] = byte(i)
	}
	for _, format := range []Encoding{Binary16, Binary32} {
		for _, end := range []byte{ZCRCE, ZCRCG, ZCRCQ, ZCRCW} {
			encoded, err := EncodeSubpacket(payload, end, format)
			if err != nil {
				t.Fatal(err)
			}
			before := bytes.Clone(encoded)
			result, used, err := DecodeSubpacket(encoded, format)
			if err != nil || used != len(encoded) || result.End != end || !bytes.Equal(result.Payload, payload) {
				t.Fatalf("subpacket %v/%v: %v", format, end, err)
			}
			if result.EndsFrame() != (end == ZCRCE || end == ZCRCW) || result.NeedsACK() != (end == ZCRCQ || end == ZCRCW) {
				t.Fatal("incorrect subpacket state")
			}
			for n := 0; n < len(encoded); n++ {
				if _, used, err := DecodeSubpacket(encoded[:n], format); !errors.Is(err, ErrIncomplete) || used != 0 {
					t.Fatalf("fragment %v/%v length %d: %d %v", format, end, n, used, err)
				}
			}
			if !bytes.Equal(before, encoded) {
				t.Fatal("subpacket decoder mutated input")
			}
			encoded[0] ^= 1
			if _, _, err := DecodeSubpacket(encoded, format); err == nil {
				t.Fatal("corrupt subpacket accepted")
			}
		}
	}
}
func TestFlowControlAndHighBitHexEnding(t *testing.T) {
	header := OffsetHeader(ZRPOS, 100)
	encoded, _ := EncodeHeader(header, Hex)
	encoded[len(encoded)-3] = 0x8d
	encoded[len(encoded)-2] = 0x8a
	if result, _, err := DecodeHeader(encoded); err != nil || result.Offset() != 100 {
		t.Fatal("lrzsz high-bit CR/LF rejected", err)
	}
	binary, _ := EncodeHeader(header, Binary32)
	injected := bytes.Clone(binary[:3])
	for _, value := range binary[3:] {
		injected = append(injected, 0x11, value, 0x93)
	}
	if result, _, err := DecodeHeader(injected); err != nil || result.Offset() != 100 {
		t.Fatal("flow control corrupted header", err)
	}
	data, _ := EncodeSubpacket([]byte("file"), ZCRCW, Binary16)
	flow := []byte{}
	for _, value := range data {
		flow = append(flow, 0x13, value, 0x91)
	}
	if result, _, err := DecodeSubpacket(flow, Binary16); err != nil || string(result.Payload) != "file" {
		t.Fatal("flow control corrupted subpacket", err)
	}
}
func TestInvalidFramesAndSizeBounds(t *testing.T) {
	header, _ := EncodeHeader(OffsetHeader(ZDATA, 1), Binary16)
	header[len(header)-1] ^= 1
	if _, _, err := DecodeHeader(header); !errors.Is(err, ErrCRC) {
		t.Fatal("bad header checksum accepted", err)
	}
	for _, value := range [][]byte{[]byte("text"), {'*', ZDLE, 'Q'}, {'*', '*', ZDLE, 'A'}} {
		if _, _, err := DecodeHeader(value); !errors.Is(err, ErrMalformed) {
			t.Fatal("invalid header accepted", value, err)
		}
	}
	if _, err := EncodeSubpacket(make([]byte, MaxSubpacket+1), ZCRCE, Binary32); !errors.Is(err, ErrTooLarge) {
		t.Fatal("oversized encoding accepted")
	}
	if _, _, err := DecodeSubpacket(bytes.Repeat([]byte{'x'}, MaxSubpacket+1), Binary16); !errors.Is(err, ErrTooLarge) {
		t.Fatal("oversized decoding accepted", err)
	}
	if _, _, err := DecodeSubpacket([]byte{ZDLE, '!'}, Binary16); !errors.Is(err, ErrMalformed) {
		t.Fatal("invalid escape accepted", err)
	}
}
