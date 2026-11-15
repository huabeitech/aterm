# Frontend

The production entry is `index.html` → `src/renderer/src/main.tsx`. The renderer is migrated from the Electron application in `~/code/aterm`; preserve its React components, layout and styles when replacing platform services.

## Boundaries

- `src/renderer/src`: React UI, renderer models and state.
- `src/renderer/src/api`: typed desktop API and Wails event adapters. Only this directory may import Wails bindings/runtime. Keep platform-independent state machines separate from runtime subscriptions so they can be tested without a webview.
- `src/shared`: shared contracts and pure helpers.
- `public`: canonical static assets; `packages`: renderer dependencies maintained in this repository.
- `bindings`: generated Go bindings, outside application source.

The inactive original Ant Design frontend has been removed. Do not add another application entry or parallel feature tree under `src`.

## Commands

Use Node.js 22.13 or later and the committed npm lockfile:

```sh
npm ci
npm test
npm run typecheck
npm run build
```

`npm ci` applies the committed xterm patch through `patch-package`. `npm test` first type-checks the tests, then runs TypeScript behavior tests with `tsx`. From the project root, run `bash scripts/check-frontend-boundaries.sh` to enforce source and runtime-import boundaries.

## Test scope

Tests of connection trees, host models, palette search, SSH command parsing, terminal helpers, workspace tabs and plugin presentation originate from the corresponding Electron behavior tests. Wails-specific tests cover keyboard routing and canonical terminal snapshot/live-output reconciliation. Selection, directory and host-mapping tests exercise the active renderer models directly.

Electron main-process tests are not copied as substitute TypeScript implementations: migrated service behavior belongs in Go tests. Removed source-text assertions against the inactive Ant Design UI do not represent acceptance coverage for this renderer. These unit checks and a successful bundle build do not establish native window, IME, file-panel, plugin or cross-platform acceptance; see `../docs/electron-parity.md` for remaining migration work.
