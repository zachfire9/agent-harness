//go:build linux || darwin || freebsd || netbsd || openbsd

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal reports whether f is an interactive terminal. Only a real TTY
// answers the terminal-attributes ioctl; files, pipes and /dev/null do not.
func isTerminal(f *os.File) bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(ioctlGetTermios), uintptr(unsafe.Pointer(&termios)))
	return errno == 0
}
