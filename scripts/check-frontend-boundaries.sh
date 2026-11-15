#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

violations="$({ rg -l "(bindings/ssh-connect|@wailsio/runtime|wailsjs/(go|runtime))" frontend/src --glob '*.{ts,tsx}' || true; } | rg -v '^frontend/src/renderer/src/api/' || true)"

if [[ -n "$violations" ]]; then
    echo "Direct Wails imports are only allowed in frontend/src/renderer/src/api/."
    echo "Move these calls behind a typed API module:"
    echo "$violations"
    exit 1
fi

unexpected="$({ rg --files frontend/src || true; } | rg -v '^frontend/src/(renderer/src|shared)/' || true)"
if [[ -n "$unexpected" ]]; then
    echo "Frontend source must live in src/renderer/src or src/shared:"
    echo "$unexpected"
    exit 1
fi

echo "Frontend module boundaries are valid."
