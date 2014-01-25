package main

import (
	"os"
	"syscall"
	"unsafe"
)

func isAtty() bool {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, os.Stdout.Fd(),
		uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return err == 0
}
