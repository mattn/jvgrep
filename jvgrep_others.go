// +build !windows,!linux,!darwin

package main

func isAtty() bool {
	return false
}
