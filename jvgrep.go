package main

import (
	"fmt"
	"github.com/mattn/go-iconv"
	"path/filepath"
	"io/ioutil"
	"os"
	"strings"
	"regexp"
)

var encodings = []string{
	"iso-2022-jp-3",
	"iso-2022-jp",
	"euc-jisx0213",
	"euc-jp",
	"utf-8",
	"ucs-bom",
	"euc-jp",
	"eucjp-ms",
	"cp932",
}

func grep(re *regexp.Regexp, path string, oc *iconv.Iconv) {
	f, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	for _, enc := range encodings {
		ic, err := iconv.Open("utf-8", enc)
		if err != nil {
			continue
		}
		did := false
		for n, line := range strings.Split(string(f), "\n") {
			t, err := ic.Conv(line)
			if err != nil {
				break
			}
			bs := re.FindAll([]byte(t), -1)
			if len(bs) == 0 {
				continue
			}
			o, err := oc.Conv(t)
			if err != nil {
				o = line
			}
			fmt.Printf("%s:%d:%s\n", path, n+1, o)
			did = true
		}
		ic.Close()
		if did {
			break
		}
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: gogrep [pattern] [file]")
		os.Exit(-1)
	}
	re, err := regexp.Compile(os.Args[1])
	if err != nil {
		println(err.String())
		os.Exit(-1)
	}
	m, err := filepath.Glob(os.Args[2])
	if err != nil {
		println(err.String())
		os.Exit(-1)
	}
	oc, err := iconv.Open("char", "utf-8")
	defer oc.Close()
	for _, f := range m {
		grep(re, f, oc)
	}
}
