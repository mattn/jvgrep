// +build !windows,!linux

package util

func IsAtty() bool {
	return false
}
