//go:build !windows

package ocr

import "os/exec"

// noWindow matters only on Windows (window_windows.go).
func noWindow(*exec.Cmd) {}
