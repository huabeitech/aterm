# Native plugins

Native plugins deliver a Go backend and a React view independently of aTerm.
They use HashiCorp go-plugin's gRPC transport, including mutual TLS and a
multiplexed reverse broker. Node.js is used only to build frontend assets.
Existing sandbox JavaScript plugins remain supported.

## Ownership and lifecycle

- A profile lock prevents two application instances from owning one plugin root.
- Each plugin ID has one persistent supervisor slot. Concurrent acquisitions share
  startup. Registry replacement, disable, rollback and uninstall drain the old
  process before committing changes. A generation identifies each instance.
- Pages are consumers, not processes. Independent business tasks live in the backend and
  survive closing pages and terminal sessions. Session-bound plugins instead
  release sampling and log streams when their last active surface closes;
  interactive child terminals are released when the source terminal closes. Commands also hold a request lease.
- With no consumers or requests, the supervisor checks `runtime.status`. A backend
  with no tasks is stopped after 30 seconds. Hidden views must stop polling.
- Crashes invalidate the backend and release its host resources. They require an
  explicit retry through plugin settings; tasks never automatically reconnect.
- Shutdown cancels host resources, asks the backend to stop (two second deadline),
  closes its lease and terminates the process tree. Windows uses a kill-on-close
  Job Object; Unix uses a private process group. The Go SDK watches its stdin
  lease so a parent crash also terminates the backend and its process group.
- Native code is trusted local code, not an OS sandbox. It can bypass SDK APIs.
  Permission declarations govern host services; they do not constrain arbitrary
  system calls. SDK lifecycle cooperation is required on Unix.

## Package and build

A native manifest has `runtime: "native"`, a `backend` with `platform` and
`sha256`, and `entry: "backend/<os>-<arch>/plugin"`. Each archive carries only
one platform binary, React HTML and locale dictionaries. Packages are bounded,
path-checked, immutable and verified again before launch. Checksums detect
modification; they do not authenticate a publisher. Installation explicitly
warns that local packages are unverified. Distribution is for the standalone
edition; Mac App Store plugin installation remains disabled.

```sh
npm --prefix frontend ci
node plugins/ssh-tunnels/build.mjs
GOOS=linux GOARCH=amd64 node plugins/ssh-tunnels/build.mjs
GOOS=windows GOARCH=amd64 node plugins/ssh-tunnels/build.mjs
```

On macOS, the default command produces both Apple Silicon (`darwin-arm64`) and
Intel (`darwin-amd64`) packages. Set `GOARCH` to build only one architecture.

Build each supported platform in CI and install its `.aterm-plugin` through
Settings → Plugins. macOS builds use the repository's pinned toolchain and are
ad hoc signed by default. Set `ATERM_PLUGIN_SIGN_IDENTITY` for distribution
signing; notarization remains a separate release step.

## Backend protocol

Import `ssh-connect/packages/plugin-go` and call `plugin.Serve(factory)`.
The factory receives a host client and returns a `Handler`:

- `Call(ctx, method, jsonArgs)` handles declared commands.
- `Open(ctx, method, jsonArgs)` handles plugin-defined streams when needed.
- `runtime.status` returns `{ "tasks": <active task count> }`.
- `runtime.shutdown` must idempotently stop all tasks and release resources.

Calls use JSON inside protobuf envelopes. Byte streams use binary gRPC frames,
backpressure, context cancellation and explicit write-half closure. The initial
stream context owns its entire lifetime. The connection adapter does not support
`SetDeadline`; callers should use context cancellation.

### Host services

| Method | Arguments / result | Permission |
| --- | --- | --- |
| `hosts.list` | Returns id, alias, hostName and user only | `hosts.read` |
| `ssh.open` | `{hostID}` → `{id}`; independent authenticated transport | `ssh.connect` |
| `ssh.tcp` (stream) | `{id,network:"tcp",address:"host:port"}` | Owned SSH handle |
| `ssh.exec` (stream) | `{id,command}`; stdin and merged stdout/stderr | Owned SSH handle |
| `ssh.wait` | `{id}`; waits for transport loss or cancellation | Owned SSH handle |
| `ssh.close` | `{id}` | Owned SSH handle |
| `storage.get` | `{key}` → JSON value or null | Plugin namespace |
| `storage.set` | `{key,value,hostIDs}`; atomic save | Plugin namespace |
| `secrets.get` | `{key}` → string | Plugin namespace; unlocked vault |
| `secrets.set` | `{key,secret}` | Plugin namespace; unlocked vault |
| `secrets.delete` | `{key}` | Plugin namespace; unlocked vault |
| `terminal.open` | `{kind:"local"}` or `{kind:"ssh",hostID}` | `terminal.open` |
| `clipboard.write` | `{text}` | Explicit plugin command |

SSH grants are scoped to the plugin executable authority and current host
connection identity. They are visible and revocable in plugin settings. Passwords, managed keys and jump-host authentication stay in aTerm.
Native stderr and go-plugin diagnostics are kept in `plugins/logs/<id>.log`
with a 64 KiB limit and private file permissions.
Every backend receives its own resource owner and a minimal environment, never
the parent application's environment or another plugin's handles.

Configuration is opaque JSON in `plugin_data`. Declared `hostIDs` protect saved
hosts from deletion. Plugin secrets use a separate namespace in the existing
encrypted vault. Both participate in encrypted backup; executables and running
processes do not. Restoring a workspace stops plugin workers before replacement.
Uninstall keeps saved data; remove host-referencing configurations in the plugin
before uninstalling, or reinstall it to edit them later.

## UI contributions

Views can declare `placement: "app.workspace"` and locations `sidebar`,
`host.details`, `connection`. An optional `icon` supplies a filled SVG path in
a 256 by 256 view box. Only path geometry is accepted, never SVG markup. Sidebar
entries use the same icon button and tooltip styling as built-in utilities. The host supplies standard entry points and opens
isolated React HTML using the existing view bridge. Commands may declare
`requiresUnlock: true` to invoke the existing vault-unlock flow before execution.
The view receives host context, visibility, theme tokens and locale updates.
Its scripts must not call Wails directly. All labels and already-open forms must
follow locale changes; raw protocol diagnostics may remain unchanged.

## Tunnel migration and verification

Migration 8 atomically converts the old tunnel table into the tunnel plugin's
`configs` data, preserves IDs and host references, then removes the old runtime
table and triggers. Old backup snapshots have a read-only conversion adapter.
The current runtime has no tunnel-specific bridge, service or page branch.

Backend tests launch the actual plugin executable and cover concurrent startup,
replacement, idle collection, crash/lease loss, binary modification, forwarding
and half-close behavior. Database/service tests cover migration, generic host
references and encrypted backup of plugin configuration and secrets.
Platform compilation alone is not native UI or process-cleanup acceptance.

## Session-bound native plugins

Docker (2.0.0) and resource-monitor (3.0.0) use `plugin.ServeSession(factory)`.
Their Go backends implement `SessionProgram`; their existing React views use the
same view bridge. A terminal status bar or a `terminal.workspace` /
`terminal.sidePanel` contribution selects the generic session adapter. No plugin
ID or business operation is embedded in that adapter.

The host issues a fresh token after execution permission is granted for a local
or SSH session. `runtime.refresh` supplies that token, host key, session label,
interval and localization bundle. `runtime.command` uses the same lease;
`runtime.stream` delivers bounded execution output. `runtime.release` cancels
in-flight sampling and commands before releasing plugin state. Late refreshes
cannot resurrect a released token. Visible sessions hold consumer leases on the
single supervised process; multiple terminals share it. After all leases and
tasks end, normal idle collection applies. The next refresh restores the current
locale and bundle even if the worker was collected in between.

Native session host services reuse the existing authorization, audit, execution
limits and local/SSH transport:

| Method | Arguments / result |
| --- | --- |
| `process.execute` | `{token,request}` → `{stdout,stderr,exitCode,truncated}` |
| `process.stream.start` | `{token,id,request}` → bounded output events |
| `process.stream.ack` / `stop` | `{token,id}` |
| `terminal.create` | `{token,request}` → `{terminalSessionId}` |
| `status.publish` | `{token,state,items,data,views}` |

Process requests require `process.execute`; interactive requests require
`terminal.create`. The manifest's execution policy still declares programs,
targets and environment overrides. Releasing a session or a crashed worker
revokes its tokens and host-owned resources. Native execution remains trusted
code, rather than an OS sandbox.

Both plugins are packaged through `scripts/build-native-plugin.mjs`, also used by
SSH tunnels. A macOS build without `GOARCH` emits both Apple Silicon and Intel
packages. CI packages all three plugins for macOS, Linux and Windows on amd64 and
arm64. Node.js is a build dependency only. The Go backend and inline React HTML
are shipped in each archive; no JavaScript backend or Node runtime is shipped.
Install the matching platform archive to update an existing script plugin; the
native-code installation confirmation is required. Execution grants are evaluated
against the new manifest policy. Monitor preferences are copied on first read
from the legacy per-plugin JSON store into `plugin_data`; no host credentials are
moved into the plugin. Five-minute monitor history is transient.

`go test ./packages/plugin-go ./internal/desktop/plugins ./plugins/...` covers
native process sharing, crash cleanup, idle restart and English/Chinese status
updates, session cancellation, Docker negotiation and engine identity, bounded
log streams, image usage, and Linux resource parsing. The opt-in Docker acceptance
test uses `ATERM_NATIVE_DOCKER_FIXTURE_IMAGE=<cached image with sh and sleep>` and
changes only its uniquely named disposable container. Platform packaging and
these tests do not establish native UI or Windows/Linux device acceptance.
