package main

import (
	"os"
	"syscall"
)

func isAtty() bool {
	var m uint32
	if syscall.GetConsoleMode(syscall.Handle(os.Stdout.Fd()), &m) == nil {
		return true
	}
	return false
}
