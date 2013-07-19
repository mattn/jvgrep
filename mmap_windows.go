package main

import (
	"os"
	"syscall"
	"unsafe"
)

type memfile uintptr

func OpenMemfile(filename string) (memfile, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	fs, err := f.Stat()
	if err != nil {
		return 0, err
	}
	fsize := fs.Size()
	fmap, err := syscall.CreateFileMapping(syscall.Handle(f.Fd()), nil, syscall.PAGE_READONLY, 0, uint32(fsize), nil)
	if err != nil {
		return 0, err
	}
	defer syscall.CloseHandle(fmap)
	mem, err := syscall.MapViewOfFile(fmap, syscall.FILE_SHARE_READ, 0, 0, uintptr(fsize))
	if err != nil {
		return 0, err
	}
	return memfile(mem), nil
}

func (mf memfile) Data() []byte {
	return *(*[]byte)(unsafe.Pointer(&mf))
}

func (mf memfile) Close() {
	syscall.UnmapViewOfFile(uintptr(mf))
}
