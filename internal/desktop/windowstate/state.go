// Package windowstate owns normal-window placement without native UI APIs.
package windowstate

import (
	"encoding/json"
	"errors"
	"math"
	"sync"
)

const MetadataKey = "desktop-main-window-v1"

type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}
type State struct {
	Rect
	Maximized bool `json:"maximized"`
}

func Valid(s State) bool {
	return s.Width >= 760 && s.Height >= 540 && s.Width <= 100000 && s.Height <= 100000 && s.X >= -1000000 && s.X <= 1000000 && s.Y >= -1000000 && s.Y <= 1000000
}
func Decode(value string) *State {
	var raw struct {
		X         *int  `json:"x"`
		Y         *int  `json:"y"`
		Width     *int  `json:"width"`
		Height    *int  `json:"height"`
		Maximized *bool `json:"maximized"`
	}
	if value == "" || json.Unmarshal([]byte(value), &raw) != nil || raw.X == nil || raw.Y == nil || raw.Width == nil || raw.Height == nil || raw.Maximized == nil {
		return nil
	}
	state := State{Rect: Rect{X: *raw.X, Y: *raw.Y, Width: *raw.Width, Height: *raw.Height}, Maximized: *raw.Maximized}
	if !Valid(state) {
		return nil
	}
	return &state
}

// Fit matches Electron window/bounds.ts: use the first intersecting display,
// otherwise the primary display (first), clamp dimensions and keep it visible.
func Fit(saved *State, displays []Rect) Rect {
	areas := make([]Rect, 0, len(displays))
	for _, area := range displays {
		if area.Width > 0 && area.Height > 0 {
			areas = append(areas, area)
		}
	}
	if len(areas) == 0 {
		areas = []Rect{{Width: 1440, Height: 900}}
	}
	area := areas[0]
	if saved != nil {
		for _, candidate := range areas {
			if saved.X < candidate.X+candidate.Width && saved.X+saved.Width > candidate.X && saved.Y < candidate.Y+candidate.Height && saved.Y+saved.Height > candidate.Y {
				area = candidate
				break
			}
		}
	}
	width, height := 1180, 780
	if saved != nil {
		width, height = saved.Width, saved.Height
	}
	width = min(width, area.Width)
	height = min(height, area.Height)
	x := area.X + int(math.Floor(float64(area.Width-width)/2+0.5))
	y := area.Y + int(math.Floor(float64(area.Height-height)/2+0.5))
	if saved != nil {
		x, y = saved.X, saved.Y
	}
	return Rect{X: max(area.X, min(x, area.X+area.Width-width)), Y: max(area.Y, min(y, area.Y+area.Height-height)), Width: width, Height: height}
}

// Tracker retains normal bounds while maximized. Minimized/fullscreen samples
// do not replace restorable geometry. All persistence and closing are serialized.
type Tracker struct {
	mu     sync.Mutex
	state  State
	save   func(State) error
	closed bool
}

func New(initial State, save func(State) error) *Tracker { return &Tracker{state: initial, save: save} }
func (t *Tracker) Observe(bounds Rect, maximized, minimized, fullscreen bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || minimized || fullscreen {
		return nil
	}
	next := t.state
	if !maximized {
		next.Rect = bounds
	}
	next.Maximized = maximized
	if !Valid(next) {
		return errors.New("invalid main window bounds")
	}
	t.state = next
	if t.save != nil {
		return t.save(next)
	}
	return nil
}
func (t *Tracker) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	if t.save != nil && Valid(t.state) {
		return t.save(t.state)
	}
	return nil
}
