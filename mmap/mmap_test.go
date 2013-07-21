package mmap

import (
	"testing"
)

func TestReadZero(t *testing.T) {
	mf, err := OpenMemfile("testdata/zero.bin")
	if err == nil {
		defer mf.Close()
		t.Fatal("should be failed for zero.bin")
	}
}

func TestReadHello(t *testing.T) {
	mf, err := OpenMemfile("testdata/hello.txt")
	if err != nil {
		t.Fatal(err);
	}
	defer mf.Close()
	b := mf.Data()
	// 12 = len("Hello World\n")
	if len(b) != 12 {
		t.Errorf("len(mf.Data()) is not 12: actually %d", len(b))
	}
	if string(b) != "Hello World\n" {
		t.Errorf("wrong contents: %s", b)
	}
}
