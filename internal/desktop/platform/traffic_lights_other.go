//go:build !darwin || !cgo

package platform

// Other platforms use their native titlebar controls without AppKit alignment.
func AlignTrafficLights(_ float64) {}
