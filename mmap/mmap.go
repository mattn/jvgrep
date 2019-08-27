// +build !windows

package mmap

import (
	"os"
	"syscall"
)

type memfile struct {
	size int64
	data []byte
}

func Open(filename string) (*memfile, error) {
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
	mem, err := syscall.Mmap(int(f.Fd()), 0, int(fsize), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return &memfile{fsize, mem}, nil
}

func (mf *memfile) Size() int64 {
	return mf.size
}

func (mf memfile) Data() []byte {
	return mf.data
}

func (mf memfile) Close() {
	syscall.Munmap(mf.data)
}
