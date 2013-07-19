// +build !windows

package main

import (
	"os"
	"syscall"
)

type memfile []byte

func OpenMemfile(filename string) (memfile, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fs, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fsize := fs.Size()
	mem, err := syscall.Mmap(int(f.Fd()), 0, int(fi.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return memfile(mem), nil
}

func (mf memfile) Data() []byte {
	return []byte(mf)
}

func (mf memfile) Close() {
	syscall.Munmap([]byte(mf))
}
