package mmap

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

type memfile struct {
	ptr  uintptr
	size int64
	data []byte
}

func Open(filename string) (mf *memfile, err error) {
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

	header := &reflect.SliceHeader{
		Data: ptr,
		Len:  int(fsize),
		Cap:  int(fsize),
	}
	return &memfile{
		ptr:  ptr,
		size: fsize,
		data: *(*[]byte)(unsafe.Pointer(header)),
	}, nil
}

func (mf *memfile) Size() int64 {
	return mf.size
}

func (mf *memfile) Data() []byte {
	return mf.data
}

func (mf *memfile) Close() {
	syscall.UnmapViewOfFile(mf.ptr)
}
