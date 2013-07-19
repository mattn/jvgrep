package main

import (
	"os"
	"syscall"
	"unsafe"
)

type memfile struct {
	ptr  uintptr
	size int64
}

func OpenMemfile(filename string) (*memfile, error) {
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
	ptr, err := syscall.MapViewOfFile(fmap, syscall.FILE_SHARE_READ, 0, 0, uintptr(fsize))
	if err != nil {
		return nil, err
	}
	return &memfile{ptr, fsize}, nil
}

func (mf *memfile) Data() []byte {
	bb := make([]byte, mf.size)
	copy(bb, *(*[]byte)(unsafe.Pointer(&mf.ptr)))
	return bb
}

func (mf *memfile) Close() {
	syscall.UnmapViewOfFile(mf.ptr)
}
