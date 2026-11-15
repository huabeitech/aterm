package zmodem

import "time"

// abortDrain discards bytes already in flight when a transfer stops. Canceling
// input ownership does not turn these bytes into terminal text. Peers end with
// CAN padding; lrzsz can then finish the interrupted subpacket's CRC trailer.
// Parsing retains only a few control bytes, never the discarded file payload.
type abortDrain struct {
	format    Encoding
	deadline  time.Time
	cans      int
	confirmed bool
	padding   bool
	flow      bool
	suffix    []byte
}

func newAbortDrain(format Encoding, peer bool) *abortDrain {
	return &abortDrain{format: format, deadline: time.Now().Add(5 * time.Second), confirmed: peer, padding: true}
}
func (d *abortDrain) consume(data []byte) (tail []byte, done bool) {
	if time.Now().After(d.deadline) {
		return data, true
	}
	for index, value := range data {
		if !d.confirmed {
			if value == ZDLE {
				d.cans++
			} else {
				d.cans = 0
			}
			if d.cans >= len(abortSequence) {
				d.confirmed = true
			}
			continue
		}
		if d.flow {
			if flowControl(value) {
				continue
			}
			return data[index:], true
		}
		if d.padding {
			if value == ZDLE || value == 8 {
				// CAN is also the start of an optional subpacket terminator. Hold one
				// until we know whether it is more padding or ZCRCE/ZCRCW plus a CRC.
				if value == ZDLE {
					d.suffix = []byte{ZDLE}
				} else {
					d.suffix = nil
				}
				continue
			}
			d.padding = false
		}
		if len(d.suffix) == 0 {
			return data[index:], true
		}
		if len(d.suffix) >= 64 {
			d.suffix = nil
			d.confirmed = false
			d.cans = 0
			d.padding = true
			continue
		}
		d.suffix = append(d.suffix, value)
		if len(d.suffix) == 2 {
			if value != ZCRCE && value != ZCRCG && value != ZCRCQ && value != ZCRCW {
				// The last CAN belonged to padding rather than a data terminator.
				return data[index:], true
			}
			if d.format == Hex {
				// Sending sessions receive headers, never data packet trailers.
				d.suffix = nil
			}
			continue
		}
		_, _, err := decodeOctets(d.suffix[2:], crcLength(d.format))
		if err == ErrIncomplete {
			continue
		}
		if err != nil {
			// Malformed post-abort control data cannot be emitted as terminal text.
			d.suffix = nil
			d.padding = true
			d.confirmed = false
			d.cans = 0
			continue
		}
		d.suffix = nil
		d.flow = true
	}
	return nil, false
}
