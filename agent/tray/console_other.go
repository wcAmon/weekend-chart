//go:build !windows

package tray

// HideConsole is a no-op on non-Windows platforms
func HideConsole() {}

// ShowConsole is a no-op on non-Windows platforms
func ShowConsole() {}
