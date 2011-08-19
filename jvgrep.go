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

type grepper struct {
	pattern string
	re      *regexp.Regexp
	oc      *iconv.Iconv
}

func (v *grepper) VisitDir(path string, f *os.FileInfo) bool {
	if path == "." {
		return true
	}
	dirmask, _ := filepath.Split(v.pattern)
	dirmask = dirmask[:len(dirmask)-1]
	path = path[:len(path)-1]
	m, e := filepath.Match(dirmask, path)
	return e == nil && m == true
}

func (v *grepper) VisitFile(path string, f *os.FileInfo) {
	dirmask, filemask := filepath.Split(v.pattern)
	dirmask = dirmask[:len(dirmask)-1]
	dir, file := filepath.Split(path)
	if dir == "" {
		if dirmask != "" {
			return
		}
	} else {
		dir = dir[:len(dir)-1]
	}

	dm, e := filepath.Match(dirmask, dir)
	if e != nil {
		return
	}
	fm, e := filepath.Match(filemask, file)
	if e != nil {
		return
	}
	if dm && fm {
		v.Grep(path)
	}
}

func (v *grepper) Grep(path string) {
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
			bs := v.re.FindAll([]byte(t), -1)
			if len(bs) == 0 {
				continue
			}
			o, err := v.oc.Conv(t)
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
	oc, err := iconv.Open("char", "utf-8")
	defer oc.Close()
	g := &grepper{os.Args[2], re, oc}
	dir, _ := filepath.Split(g.pattern)
	if len(dir) > 0 && dir[0] == '*' {
		dir = "."
	}
	filepath.Walk(dir, g, nil)
}
