package zmodem

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestAbortDrainDiscardsInflightDataAndOptionalCRCTrailer(t *testing.T) {
	for _, format := range []Encoding{Hex, Binary16, Binary32} {
		for _, trailer := range []bool{false, true} {
			if format == Hex && trailer {
				continue
			}
			wire := bytes.Repeat([]byte{0, 1, 2, 3, 24, 88}, 1024)
			wire = append(wire, bytes.Repeat([]byte{ZDLE}, 10)...)
			wire = append(wire, bytes.Repeat([]byte{8}, 10)...)
			if trailer {
				packet := mustPacket(t, []byte("fixture"), ZCRCE, format)
				end := bytes.Index(packet, []byte{ZDLE, ZCRCE})
				if end < 0 {
					t.Fatal("missing packet terminator")
				}
				wire = append(wire, packet[end:]...)
				wire = append(wire, 0x11)
			}
			prefix := len(wire)
			wire = append(wire, []byte("fixture 🌏 $ ")...)
			// Split each byte of the abort marker, padding, terminator and CRC; file
			// payload before that boundary can be arbitrary and is never retained.
			for split := prefix - 40; split <= len(wire); split++ {
				drain := newAbortDrain(format, false)
				var text bytes.Buffer
				first, done := drain.consume(wire[:split])
				text.Write(first)
				if done {
					text.Write(wire[split:])
				} else {
					tail, done := drain.consume(wire[split:])
					if !done {
						t.Fatal("shell output did not resume", format, trailer, split)
					}
					text.Write(tail)
				}
				if text.String() != "fixture 🌏 $ " {
					t.Fatalf("drain altered shell output or exposed protocol format=%c trailer=%t split=%d: %x", format, trailer, split, text.Bytes())
				}
			}
		}
	}
}
func TestAbortDrainBoundsControlFragmentsAndExpires(t *testing.T) {
	drain := newAbortDrain(Binary32, true)
	wire := append(bytes.Repeat([]byte{8}, 10), ZDLE, ZCRCE)
	wire = append(wire, bytes.Repeat([]byte{0x11}, 10000)...)
	for _, value := range wire {
		if _, done := drain.consume([]byte{value}); done {
			t.Fatal("malformed control became text")
		}
		if len(drain.suffix) > 64 {
			t.Fatal("unbounded control suffix")
		}
	}
	drain.deadline = time.Now().Add(-time.Second)
	tail, done := drain.consume([]byte("shell prompt"))
	if !done || string(tail) != "shell prompt" {
		t.Fatal("drain did not expire", done, string(tail))
	}
}
func TestRouterCanceledTransferDrainsAcrossOutputReads(t *testing.T) {
	var text bytes.Buffer
	router := NewRouter(context.Background(), RouterOptions{Write: func([]byte) error { return nil }, Output: func(data []byte) { text.Write(data) }, Offer: func(context.Context, FileOffer) (Download, error) { return &stagedDownload{}, nil }})
	defer router.Close()
	router.Consume(mustHeader(t, ZRQINIT, 0, Hex))
	router.Consume(offerWire(t, 100000, Binary32))
	router.Consume(mustHeader(t, ZDATA, 0, Binary32))
	_ = router.Cancel()
	if router.Busy() {
		t.Fatal("cancel retained input lock")
	}
	packet := mustPacket(t, []byte("remaining file data"), ZCRCE, Binary32)
	wire := append([]byte{0, 1, 2, 3}, bytes.Repeat([]byte{ZDLE}, 10)...)
	wire = append(wire, bytes.Repeat([]byte{8}, 10)...)
	wire = append(wire, packet[bytes.Index(packet, []byte{ZDLE, ZCRCE}):]...)
	wire = append(wire, 0x11)
	wire = append(wire, []byte("ordinary prompt\r\n")...)
	for _, value := range wire {
		router.Consume([]byte{value})
	}
	if text.String() != "ordinary prompt\r\n" {
		t.Fatalf("canceled wire leaked or prompt disappeared: %x", text.Bytes())
	}
	// A subsequent transfer must be detected normally rather than discarded.
	router.Consume(receiveTranscript(t, []byte("next file")))
	if router.Busy() {
		t.Fatal("next transfer could not finish")
	}
}
