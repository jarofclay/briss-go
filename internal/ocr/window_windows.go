package ocr

import (
	"os/exec"
	"syscall"
)

// noWindow starts Tesseract without a console window. The desktop app has no
// console of its own, so Windows would otherwise open (and flash) one for
// every page it reads.
func noWindow(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000 // CREATE_NO_WINDOW
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
