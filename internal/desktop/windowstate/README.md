# Main window placement

`Fit` ports Electron's `src/main/window/bounds.ts` algorithm without importing
native UI types. Displays are passed with the primary first and use DIP work-area
coordinates. A saved window stays on its first intersecting display; a removed
display falls back to the primary. Dimensions and position are clamped to that
work area, and the initial size is 1180 × 780. The Wails adapter applies the same
1100 × 680 minimum as Electron's BrowserWindow constructor after fitting.

`State` stores normal bounds and the maximized flag. `Decode` rejects incomplete,
null, malformed and implausible geometry. `Tracker` keeps the last normal bounds
when maximized, ignores minimized/fullscreen samples, serializes persistence and
prevents writes after close. Geometry belongs to the application profile's
`desktop-main-window-v1` metadata, separately from preferences and host backups.

The native adapter restores once before showing the main window, samples settled
native geometry after a 250 ms move/resize/zoom debounce, captures an accepted
window-close request and flushes cached state before storage shutdown. It never
holds its timer/state mutex while dispatching synchronous native getters. The
old startup operation that expanded every launch to a full display was removed.
React components, styling and layout are unchanged.

Validation includes 103 comparisons against Electron's actual fitting function,
removed/negative-coordinate displays, corrupt metadata and concurrent tracker
save/close tests with the race detector. Enable the independent oracle with:

```sh
ATERM_ELECTRON_REFERENCE_DIR=/path/to/aterm go test -race ./internal/desktop/windowstate .
```

Native restart/zoom/fullscreen/multi-display acceptance remains pending: computer
control currently cannot obtain a window for Finder, so native interactions were
not exercised. The Wails framework does not expose Electron's getNormalBounds;
normal bounds are retained from the last settled normal sample. Rapid resize to
maximize transitions and quit during the debounce need explicit GUI verification.
Cross-compilation of this pure package is not native Windows acceptance.
