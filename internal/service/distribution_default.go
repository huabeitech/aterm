//go:build !mas

package service

const macAppStoreBuild = false

func LocalTerminalAvailable() bool { return true }
