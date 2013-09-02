// +build !windows,!linux

package main

func isAtty() bool {
	return false
}
