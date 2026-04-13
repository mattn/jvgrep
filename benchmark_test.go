package main

import (
	"bytes"
	"strings"
	"testing"
)

func benchmarkData() []byte {
	line := "INFO alpha beta gamma delta ERROR target_token omega sigma tau\n"
	return bytes.Repeat([]byte(line), 4096)
}

func BenchmarkDoGrepFixedUTF8(b *testing.B) {
	data := benchmarkData()
	arg := &GrepArg{
		pattern: "target_token",
		needle:  []byte("target_token"),
		ascii:   true,
	}

	ignorebinary = false
	ignorecase = false
	only = false
	list = false
	invert = false
	count = false
	number = false

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		arg.buf.Reset()
		doGrepFixedUTF8("bench.txt", data, arg, arg.needle)
	}
}

func BenchmarkDoGrepFixedUTF8IgnoreCaseASCII(b *testing.B) {
	data := []byte(strings.ReplaceAll(string(benchmarkData()), "target_token", "TaRgEt_ToKeN"))
	arg := &GrepArg{
		pattern: "target_token",
		needle:  []byte("target_token"),
		folded:  []byte("target_token"),
		ascii:   true,
	}

	ignorebinary = false
	ignorecase = true
	only = false
	list = false
	invert = false
	count = false
	number = false

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		arg.buf.Reset()
		doGrepFixedUTF8FoldASCII("bench.txt", data, arg, arg.folded)
	}
}
