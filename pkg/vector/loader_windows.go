//go:build windows

package vector

import "syscall"

func load(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}
