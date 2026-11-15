package zmodem

import "testing"

func FuzzWireDecoders(f *testing.F) {
	header, _ := EncodeHeader(OffsetHeader(ZDATA, 0xff181100), Hex)
	packet, _ := EncodeSubpacket([]byte{0, 17, 24, 127, 255}, ZCRCW, Binary32)
	f.Add(header, byte(0))
	f.Add(packet, byte(2))
	f.Add([]byte{ZDLE}, byte(1))
	f.Add([]byte("ordinary terminal output"), byte(0))
	f.Fuzz(func(t *testing.T, input []byte, mode byte) {
		if len(input) > 2*MaxSubpacket+32 {
			t.Skip()
		}
		if _, used, err := DecodeHeader(input); err == nil && (used <= 0 || used > len(input)) {
			t.Fatal("invalid header consumption", used, len(input))
		}
		format := []Encoding{Hex, Binary16, Binary32}[mode%3]
		if packet, used, err := DecodeSubpacket(input, format); err == nil && (used <= 0 || used > len(input) || len(packet.Payload) > MaxSubpacket || !validEnd(packet.End)) {
			t.Fatal("invalid subpacket result")
		}
	})
}
