//go:build windows

// Package sysproxy toggles the operating system's HTTP proxy setting.
package sysproxy

import (
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const internetSettings = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// Bypass list: loopback, private ranges and local names never use the proxy.
const bypass = "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"

var (
	wininet           = syscall.NewLazyDLL("wininet.dll")
	internetSetOption = wininet.NewProc("InternetSetOptionW")
)

const (
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
)

// Supported reports whether this platform can set a system proxy.
func Supported() bool { return true }

// Enable points the per-user WinINet proxy at server ("host:port").
func Enable(server string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettings, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("ProxyServer", server); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyOverride", bypass); err != nil {
		return err
	}
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	notify()
	return nil
}

// Disable turns the system proxy off, but only if it still points at server,
// so a proxy the user configured elsewhere is left alone.
func Disable(server string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettings, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if current, _, err := k.GetStringValue("ProxyServer"); err == nil && current != server {
		return nil
	}
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return err
	}
	notify()
	return nil
}

func notify() {
	internetSetOption.Call(0, internetOptionSettingsChanged, 0, 0)
	internetSetOption.Call(0, internetOptionRefresh, 0, 0)
}
