//go:build !windows

// Package sysproxy toggles the operating system's HTTP proxy setting.
package sysproxy

import "errors"

// Supported reports whether this platform can set a system proxy.
func Supported() bool { return false }

// Enable is not implemented on this platform yet.
func Enable(string) error { return errors.New("system proxy is not supported on this platform yet") }

// Disable is a no-op on this platform.
func Disable(string) error { return nil }
