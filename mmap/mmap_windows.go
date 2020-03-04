package mmap

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// Memfile is
type Memfile struct {
	ptr  uintptr
	size int64
	data []byte
}

// Open filename with mmap
func Open(filename string) (mf *Memfile, err error) {
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
	fmap, err := syscall.CreateFileMapping(syscall.Handle(f.Fd()), nil, syscall.PAGE_READONLY, 0, uint32(fsize), nil)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(fmap)
	ptr, err := syscall.MapViewOfFile(fmap, syscall.FILE_MAP_READ, 0, 0, uintptr(fsize))
	if err != nil {
		return nil, err
	}
	defer func() {
		if recover() != nil {
			mf = nil
			err = errors.New("Failed option a file")
		}
	}()
	return &Memfile{ptr, fsize, (*[1 << 30]byte)(unsafe.Pointer(ptr))[:fsize]}, nil
}

// Size return a size of data
func (mf *Memfile) Size() int64 {
	return mf.size
}

// Data return the bytes
func (mf *Memfile) Data() []byte {
	return mf.data
}

// Close the memfile
func (mf *Memfile) Close() {
	syscall.UnmapViewOfFile(mf.ptr)
}
