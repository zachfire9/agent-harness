//go:build windows

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
)

// isTerminal reports whether f is an interactive console. Redirected input
// (files, pipes, NUL) has no console mode.
func isTerminal(f *os.File) bool {
	var mode uint32
	ok, _, _ := procGetConsoleMode.Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
	return ok != 0
}
