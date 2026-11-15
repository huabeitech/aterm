# Shared SSH-agent transport

`Dial` is the common transport boundary for native SSH authentication, explicit
agent forwarding and adding a protected managed key to an agent. It returns a
caller-owned `net.Conn`; the SSH agent protocol stays in x/crypto/ssh/agent.
The discovery context has a five-second cap and does not close a successful
connection when Dial returns. Operation deadlines and lifecycle cleanup remain
with each caller.

An explicit SSH_AUTH_SOCK is honored. Unix uses AF_UNIX. Windows defaults to
`\\.\pipe\openssh-ssh-agent` and uses Microsoft's go-winio DialPipeContext for
named-pipe endpoints; an explicit AF_UNIX path remains supported as a transport
choice. The dependency is pinned to a release compatible with the project's
existing Go version; the Go language/toolchain requirement was not raised.

Authentication still bounds key discovery separately and closes that connection
after handshake. Forwarding owns a separate connection until the SSH client
closes. Managed-key addition decrypts the selected key in memory, verifies its
catalog fingerprint and closes/cancels its connection without writing private
material to a temporary file.

## Evidence and remaining work

Unix tests use a disposable socket and real agent keyring to list a fixture key,
request a signature, verify it and prove ownership/close behavior. Existing real
agent-forwarding signatures, handshake/discovery cancellation, rejected forwarding
and protected managed-key addition passed after using the shared transport.
Endpoint override/cancellation checks also pass with the race detector.

Windows-only tests create a private pipe and exercise the same list/sign/verify/
close protocol, never the system agent pipe. Their test binary and the service
and managed-key package test binaries cross-compiled successfully on macOS.
**These Windows tests have not run on Windows.** System OpenSSH agent ACL,
availability/busy-pipe handling and authentication/forwarding/key-add UI require
native Windows acceptance.

## Pageant

When default Windows OpenSSH discovery reports that its pipe is absent, Dial now
falls back to Pageant. An explicit endpoint is honored; a busy/timed-out pipe is
not treated as absent. Explicit `pageant` also selects that transport. Pageant
connections are used for authentication and forwarding. Adding managed keys to
Pageant remains unavailable, matching Electron's restriction; that operation
requires OpenSSH agent and does not transmit the selected private key to Pageant.

Pageant IPC uses bounded frames (8 KiB), a random mapping name per request and a
mapping owned/access-controlled to the current user. The target must belong to a
different process. Windows messages use SendMessageTimeoutW with a nonzero bounded
wait, not broadcast. Calls pin the OS thread while clearing/reading last-error
state. See [Microsoft's timeout API documentation](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendmessagetimeoutw).
The shared-memory protocol reference is [go-pageant v1.0.2](https://github.com/davidmz/go-pageant/tree/v1.0.2);
its license notice is retained in PAGEANT-LICENSE.txt. It is not an added Go module
dependency.

A separate net.Pipe-backed request/response bridge supports x/crypto's pipelined
agent reader, fragmented writes, backpressure, deadlines and close wakeups without
an unbounded RPC queue. One worker retains only its bounded request/reply. Native
queries have a 30-second cap and use the earlier read deadline when present;
at most eight native calls can be active. Closing the stream cancels queued work
and lets callers return promptly. A native Windows message already in progress
can finish later within its submitted timeout; mapping handles remain owned until
that call returns. Extending a stream deadline does not extend an already sent
native IPC message.

Protocol tests use a real Go agent keyring, including sixteen concurrent
list/sign/verify callers. They cover a reader started before its request,
fragmented frames, malformed/oversized frames, typed raw-stream timeouts, agent API
timeout messages, cancellation and worker cleanup. The agent API formats transport
errors as text; the raw stream retains structured timeout errors. Existing Unix
agent/key/forwarding regressions pass with the race detector.

Windows tests cover COPYDATASTRUCT ABI, invalid-frame rejection before native
calls and cancellation while all native-call slots are occupied. The agent test
binaries cross-compiled for 32-bit/64-bit Windows and the service compiled for
64-bit Windows. **No Windows test binary has been executed here.** Actual Pageant
signing/forwarding, shared-memory ACL ownership, user-approval dialogs, hung-process
behavior and fallback discovery remain pending native acceptance. This source
implementation and cross-compilation do not establish complete Windows parity.
