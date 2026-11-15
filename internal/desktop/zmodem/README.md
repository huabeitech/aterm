# Native ZMODEM boundary

This package keeps terminal transfer parsing out of React and out of xterm's
rendering/acknowledgement path. The wire codec is implemented in Go; the existing
Electron dependency is used only as a test oracle.

`DecodeHeader` and `DecodeSubpacket` accept arbitrary terminal read fragments.
They return `ErrIncomplete` with zero bytes consumed until the entire frame and
checksum are available. Neither function mutates its input. A successful result
reports exactly how many wire bytes were consumed, leaving following frames for
the caller. Bad CRCs, invalid framing/escapes and oversized subpackets are
separate errors; an incomplete fragment must never be treated as corrupt data.

Headers support hexadecimal/CRC16, binary/CRC16 and binary/CRC32. Subpackets
preserve all byte values and expose frame-end and acknowledgement requirements.
Encoding always escapes control characters; decoding also accepts ZRUB0/ZRUB1
and link-level XON/XOFF. Data subpackets are limited to 1 MiB. Header offsets use
the protocol's 32-bit little-endian representation.

Validation:

```sh
go test -race ./internal/desktop/zmodem
ATERM_ELECTRON_REFERENCE_DIR=/path/to/aterm go test ./internal/desktop/zmodem
go test ./internal/desktop/zmodem -run '^$' -fuzz FuzzWireDecoders -fuzztime=3s -parallel=2
```

The interoperability test executes Electron's installed zmodem.js-ex 3.0.0
codec, verifies that it accepts Go output, compares encoded bytes, and decodes
its output in Go. It covers all three header encodings and all four subpacket
terminators, including empty packets and payloads containing every byte value.

## Receiving runtime

`Router` separates transfer frames from canonical terminal text before replay,
UTF-8 decoding, renderer delivery and acknowledgement accounting. It detects
ZRQINIT for downloads and ZRINIT for uploads, and owns the active session,
cancellation and inactivity deadline.
`Receiver` handles fragmented handshakes, offers, CRC16/CRC32 data, offset and
size validation, batch files, acknowledgements and the ZFIN/OO finish exchange.
Only ordinary text before and after a transfer reaches the terminal renderer.

The application supplies an asynchronous native save chooser through `Offer`.
`NewDownload` opens a private sibling staging file and publishes it only after a
valid EOF, using a non-overwriting link. Cancellation or failure removes the
partial file. Existing destinations and late collisions are rejected. Each file
commits independently; a later failure does not remove earlier completed files.
Protocol, file ownership and native UI integration remain separate boundaries.

The existing React terminal progress contract receives download events. Normal
text and binary input are suppressed during transfer; text Ctrl+C and the
existing cancel action stop the session. Canceling a pending chooser prevents a
late selection from creating a file, although the OS panel may still require
closing. No protocol parser or whole-file buffering was added to React.

Additional evidence includes a real Electron JavaScript sender delivering all
byte values through pipes and through a local PTY into a staged file, with exact
file comparison and proof that binary frames do not enter canonical replay.
Tests cover batches, empty files, partial writes, CRC/offset/size failures,
cancellation during selection, timeouts, collision races and trailing text.
`scripts/zmodem-peer.cjs` is a test-only Electron protocol peer; it accepts only
explicit temporary fixture files and restores terminal settings on exit.

## Sending runtime

`Sender` negotiates the peer capabilities and control escaping, then requests
native multi-file selection through `Select`. `OpenUploads` verifies regular
files and retains descriptors rather than reopening paths during transmission.
A path replacement cannot switch the source. Source descriptors close after EOF
acknowledgement, skip, cancellation, timeout or any failure. Unsupported source
sizes above the protocol's 32-bit offset range are rejected.

Data streams in 8 KiB packets with at most 1 MiB sent before a ZCRCQ acknowledgement
is required. Intermediate packets keep the ZDATA frame open; final packets end
it before ZEOF. Each encoded packet is written directly to the transport rather
than retained in an output queue. This bounds memory and in-flight data while
avoiding a round trip for every small packet. ZRPOS supports initial resume and
bounded retry; acknowledgements and offsets are validated. File metadata includes
size, mtime, remaining file count and remaining byte count. Batch skips and empty
files proceed to the next offer and the final ZFIN/OO exchange.

The Wails adapter asynchronously presents the native multi-file picker. A late
result after cancellation cannot open sources. The existing React progress,
input guard and cancel action work for either direction through a shared session
boundary. Raw transport writes now retain their lock across partial writes and
report zero writes instead of silently losing frame bytes.

Validation executes Electron's real receiving session with all-byte fixtures,
a batch containing a skipped file and an empty file, and resume at zero, a
nonzero offset and EOF. A separate local PTY test verifies the active terminal
adapter, exact received file and binary-free canonical replay. Tests also cover
CRC16/CRC32 fragmentation, bounded acknowledgement windows, invalid offsets,
source/read/write failures, late selection, cancellation, timeout and descriptor
cleanup. `scripts/zmodem-receive-peer.cjs` is a disposable receiving oracle, not
a production dependency. Windows package cross-compilation passed; Windows
native file/PTY behavior has not been exercised.

## Independent tool and SSH validation

Opt-in tests can also use an independently obtained lrzsz installation:

```sh
ATERM_LRZSZ_DIR=/path/to/lrzsz/bin go test -race ./internal/desktop/zmodem .
```

`lrzsz_interop_test.go` runs actual lsz/lrz processes. Successful cases transfer
2 MiB all-byte fixtures plus empty batch files; downloads exercise CRC16, CRC32
and synchronous per-packet acknowledgements, while uploads exercise default and
explicit control escaping. Separate 4 MiB fixtures stop mid-transfer and verify
that local staging files, source descriptors and remote incomplete files are
cleaned up. The Unix root test runs the native SSH service/terminal manager over
a public-key-authenticated loopback SSH connection to the same tools on a real
PTY, checking exact bytes in both directions and binary-free terminal replay.
This fixture is not an external sshd or a native GUI acceptance test.

Cancellation keeps a small, separate stream drain after releasing terminal input
ownership. An independent lsz process exposed roughly 64 KiB of queued payload
that previously entered replay after cancellation. `abortDrain` now discards that
payload through the peer's CAN/backspace acknowledgement and optional interrupted
subpacket CRC trailer. It preserves following ordinary text across arbitrary
read boundaries and retains at most 64 bytes of control suffix. A five-second
fallback ends draining if the peer never acknowledges; it cannot prove a broken
peer has stopped sending data. The tests cover fragmented padding/CRC/flow bytes,
post-cancel prompt text, a subsequent transfer, memory bounds and expiry.

The independently fetched tools are development fixtures only. They were unpacked
under a temporary directory, not installed into the app or added as dependencies.

## Remaining acceptance

Native save/open-panel interaction, visible progress and GUI cancellation still
need end-to-end acceptance. The latest attempt could not obtain accessible
windows for either the fixture application or Finder; this does not establish an
application-specific cause or validate a panel. External sshd variants, wide-area
latency/loss, forced transport disconnects and native Windows/Linux behavior remain
unverified. The completed tests establish codec/tool/PTY/SSH behavior, not complete
Electron feature parity. The legacy whole-file helpers are not this runtime.
