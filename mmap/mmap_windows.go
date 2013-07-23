package mmap

import (
	"os"
	"syscall"
	"unsafe"
)

type memfile struct {
	ptr  uintptr
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
	fmap, err := syscall.CreateFileMapping(syscall.Handle(f.Fd()), nil, syscall.PAGE_READONLY, 0, uint32(fsize), nil)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(fmap)
	ptr, err := syscall.MapViewOfFile(fmap, syscall.FILE_MAP_READ, 0, 0, uintptr(fsize))
	if err != nil {
		return nil, err
	}
	data := make([]byte, fsize)
	copy(data, (*(*[]byte)(unsafe.Pointer(&ptr)))[:fsize])
	return &memfile{ptr, data}, nil
}

func (mf *memfile) Data() []byte {
	return mf.data
}

func (mf *memfile) Close() {
	syscall.UnmapViewOfFile(mf.ptr)
}
