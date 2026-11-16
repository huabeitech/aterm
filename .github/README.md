# GitHub Actions

## CI

`ci.yml` runs on every pull request, pushes to `main`, and manual dispatch. It checks Go formatting, generates Wails bindings and embedded frontend assets, runs the existing backend/frontend test task, and runs Go vet. It deliberately has no path filter so `Code checks` can be configured as a required branch check without leaving documentation-only PRs pending.

## Test builds

`build.yml` runs on pushes to `main` except documentation-only changes, and manual dispatch. Once the workflow exists on the default branch, choose **Actions → Build → Run workflow** and select a branch to build it.

The Build workflow produces separate app ZIPs for Apple Silicon (`arm64`) and Intel (`x86_64`). Each job builds the app with the existing Taskfile and checks the binary architecture before packaging. Download the artifact for your Mac from the workflow run. Extract the outer artifact first, verify the app ZIP with `shasum -a 256 -c <archive>.sha256`, then extract the app ZIP to preserve the executable permissions and bundle structure. Artifacts expire after 14 days.

These are ad-hoc signed test builds. They are not notarized public releases or App Store packages. The workflows do not require signing secrets, change version numbers, create releases, or upload to App Store Connect.

## Toolchain and operation

Both workflows use the shared local setup action, the exact Go toolchain in `build/config.yml`, the Wails version in `go.mod`, and Node.js 22.13.0. CI runs on `macos-15`. Build runs on `macos-15` for Apple Silicon and `macos-15-intel` for Intel. Dependencies use the committed npm lockfile and Go checksums. Third-party actions are pinned to commit SHAs; Go and npm download caches are enabled. New runs cancel obsolete runs for the same PR or branch. Each job has a 30-minute timeout and read-only repository permissions.

For local validation, install the matching Wails CLI, run `wails3 task frontend:install`, `wails3 task generate:bindings`, `wails3 task test`, `python3 scripts/go-macos.py vet -copylocks=false ./...`, then `wails3 task build`. Run checks and builds sequentially because they share generated bindings and frontend assets.

Repository administrators can require `Code checks` in branch rules after the first successful GitHub run. Developer ID distribution, notarization, App Store packaging/upload, and additional platforms remain separate follow-up workflows.

Go vet currently excludes `copylocks`: the existing terminal Session snapshot API returns a struct containing `sync.Mutex`, which produces baseline warnings. Other vet analyzers remain enabled. Resolve that API issue separately before enabling this analyzer.
