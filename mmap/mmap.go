// +build !windows

package mmap

import (
	"os"
	"syscall"
)

type Memfile struct {
	size int64
	data []byte
}

// Open filename with mmap
func Open(filename string) (*Memfile, error) {
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
	return &Memfile{fsize, mem}, nil
}

// Size return a size of data
func (mf *Memfile) Size() int64 {
	return mf.size
}

// Data return the bytes
func (mf Memfile) Data() []byte {
	return mf.data
}

// Close the memfile
func (mf Memfile) Close() {
	syscall.Munmap(mf.data)
}
