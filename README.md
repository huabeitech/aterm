# aTerm — Wails

A Go/Wails v3 desktop application using the React renderer migrated from the Electron project at `../aterm`. The migration aims to preserve its UI and behavior while replacing Electron services with native Go implementations. Full parity and platform acceptance are still in progress.

## Development

Use Go 1.25+, Node.js 22.13+, npm, Wails v3 and the native build tools for your platform. The current application-bundle tasks target macOS.

```sh
wails3 task deps
wails3 task dev
wails3 task test
wails3 task build
```

The macOS build produces `build/bin/aTerm.app`. The build uses the committed npm lockfile, reapplies the xterm compatibility patch, generates Go bindings, builds the renderer and signs the local bundle ad hoc. It does not publish or upload a release. Release signing requires the separate configured distribution workflow.

## Architecture

- `frontend/src/renderer/src`: migrated React UI, renderer models and state.
- `frontend/src/renderer/src/api`: typed Wails adapters; direct runtime/binding imports stay here.
- `frontend/src/shared`: shared renderer contracts and pure helpers.
- `main.go`: embeds the frontend assets and calls the application entry point.
- `internal/app`: Wails lifecycle, dependency composition, windows and desktop contract adapters.
- `internal/terminal`: terminal sessions, output flow control, SSH/local processes and transfer coordination.
- `internal/desktop/platform`: build-tagged native window/input/accessibility bridges and their Objective-C implementations.
- `internal/desktop`: focused native runtime modules, including files, plugins, keys, terminal metadata, local PTYs and transfers.
- `internal/service`, `internal/db`, `internal/ssh`: application services, persistent data and SSH transport.

See [frontend ownership and tests](frontend/README.md) and the module READMEs for lifecycle boundaries. React components should remain independent of Wails APIs.

## Product identity and data

`build/config.yml` is the canonical product name/version/build-number and macOS build-target configuration. The build derives artifact names from it and runs `scripts/sync-build-metadata.py` to synchronize the plist and `internal/desktop/product/info.json`; the latter is embedded for runtime/bootstrap identity. When changing metadata outside the build task, run:

```sh
python3 scripts/sync-build-metadata.py --plist build/darwin/Info.plist --plist build/darwin/Info.dev.plist --runtime-json internal/desktop/product/info.json
```

macOS builds/tests use the pinned Go 1.26.11 toolchain (downloaded through Go's toolchain mechanism), native compilation flags and a macOS 12.0 deployment target. The system Go selection is not changed. The normal build checks the actual Mach-O minimum version, embedded toolchain, plist identity and local signature. Go 1.27 requires macOS 13 and must not be used to produce a package declaring macOS 12 support. A plain `go build` outside the helper does not provide that shipping guarantee. Actual macOS 12 runtime acceptance remains pending.

Existing `TermSSH` storage directories, legacy backup metadata and Keychain service identifiers retain their compatibility meaning. Renaming the application is not a data migration; do not rename those paths without an explicit migration. For isolated development, set `ATERM_DATA_DIR` to an absolute disposable directory before launch.

## Validation limits

Keep `wails3 task test` separate from a concurrent `wails3 task build`: the build replaces npm dependencies and embedded renderer assets while Go scans/embeds project files. The test task itself sequences its checks.

Go/renderer tests and cross-compilation provide scoped implementation evidence. They do not establish native UI, IME, signing/distribution or Windows/Linux runtime acceptance.

## Private development and release material

Keep release signing material outside the repository. See [Mac App Store signing setup](build/darwin/README.md) for the local signing directory and environment overrides.

Never commit real SSH connection details, passwords, private keys, local environment credentials, user databases, backups, or debug logs. Use synthetic hosts and credentials in test fixtures. Ignored paths do not protect files that Git already tracks; review staged changes before committing.

For disposable local data and diagnostics, use `.local/`, `build/local-data/`, `build/backups/`, or `build/debug-logs/`. Set `ATERM_DATA_DIR` to an absolute path when isolating application data.
