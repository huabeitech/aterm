//go:build !darwin || !cgo

package platform

func AccessibilityEnabled() bool             { return false }
func ObserveAccessibility(func(bool)) func() { return func() {} }
