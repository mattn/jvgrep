package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/mattn/go-iconv"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

var encodings = []string{
	"iso-2022-jp-3",
	"iso-2022-jp",
	"euc-jisx0213",
	"euc-jp",
	"utf-8",
	"ucs-2",
	"ucs-2be",
	"euc-jp",
	"eucjp-ms",
	"cp932",
}

type grepper struct {
	pattern string
	re      *regexp.Regexp
	ere     *regexp.Regexp
	oc      *iconv.Iconv
}

func (v *grepper) VisitDir(dir string, f *os.FileInfo) bool {
	if dir == "." || *recursive {
		return true
	}
	dirmask, _ := filepath.Split(v.pattern)
	dir = filepath.ToSlash(dir)

	mi := strings.Split(dirmask, "/")
	if len(mi) == 2 && mi[0] == "**" {
		if m, e := filepath.Match(dirmask, dir); e != nil || m == false {
			return true
		}
	}
	for i, d := range strings.Split(dir, "/") {
		if len(mi) <= i {
			break
		}
		if m, e := filepath.Match(mi[i], d); e != nil || m == false {
			return false
		}
	}

	return true
}

func (v *grepper) VisitFile(path string, f *os.FileInfo) {
	if v.ere != nil && v.ere.MatchString(path) {
		return
	}
	dirmask, filemask := filepath.Split(v.pattern)
	dir, file := filepath.Split(path)

	dirmask = filepath.ToSlash(dirmask)
	if dirmask == "**/" || *recursive {
		dir = dirmask
	} else {
		dir = filepath.ToSlash(dir)
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
		if *verbose {
			println("search:", path)
		}
		v.Grep(filepath.ToSlash(path))
	}
}

func (v *grepper) Grep(input interface{}) {
	var f []byte
	var path = ""
	var ok bool
	var stdin *os.File
	var err os.Error

	if path, ok = input.(string); ok {
		f, err = ioutil.ReadFile(path)
		if err != nil {
			return
		}
	} else if stdin, ok = input.(*os.File); ok {
		f, err = ioutil.ReadAll(stdin)
		if err != nil {
			return
		}
		path = "stdin"
	}
	for _, enc := range encodings {
		ic, err := iconv.Open("utf-8", enc)
		if err != nil {
			continue
		}
		did := false
		for n, line := range bytes.Split(f, []byte{'\n'}) {
			t, err := ic.ConvBytes(line)
			if err != nil {
				break
			}
			fi := v.re.FindAllIndex(t, -1)
			c := len(fi)
			if (!*invert && c == 0) || (*invert && c > 0) {
				continue
			}
			if *verbose {
				println("found("+enc+"):", path)
			}
			if *list {
				fmt.Println(path)
				did = true
				break
			}
			o, err := v.oc.ConvBytes(t)
			if err != nil {
				o = line
			}
			fmt.Printf("%s:%d:%s\n", path, n+1, o)
			did = true
		}
		ic.Close()
		runtime.GC()
		if did {
			break
		}
	}
}

var encs = flag.String("enc", "", "encodings: comma separated")
var recursive = flag.Bool("R", false, "recursive")
var list = flag.Bool("l", false, "listing files")
var invert = flag.Bool("v", false, "invert match")
var verbose = flag.Bool("S", false, "verbose")
var exclude = flag.String("exclude", "", "exclude files: specify as regexp")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: jvgrep [options] [pattern] [file...]\n")
		fmt.Fprintln(os.Stderr, "  Version 0.2")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Supported Encodings:")
		for _, enc := range encodings {
			fmt.Fprintln(os.Stderr, "    " + enc)
		}
		os.Exit(-1)
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
	}
	re, err := regexp.Compile(flag.Arg(0))
	if err != nil {
		println(err.String())
		os.Exit(-1)
	}
	var ere *regexp.Regexp
	if *exclude != "" {
		ere, err = regexp.Compile(*exclude)
		if err != nil {
			println(err.String())
			os.Exit(-1)
		}
	}
	if *encs != "" {
		encodings = strings.Split(*encs, ",")
	} else {
		enc_env := os.Getenv("JVGREP_ENCODINGS")
		if enc_env != "" {
			encodings = strings.Split(enc_env, ",")
		}
	}

	oc, err := iconv.Open("char", "utf-8")
	if err != nil {
		oc, err = iconv.Open("utf-8", "utf-8")
	}
	defer func() {
		if oc != nil {
			oc.Close()
		}
	}()

	if flag.NArg() == 1 {
		g := &grepper{"", re, ere, oc}
		g.Grep(os.Stdin)
	} else {
		for _, arg := range flag.Args()[1:] {
			g := &grepper{filepath.ToSlash(arg), re, ere, oc}

			root := ""
			for _, i := range strings.Split(g.pattern, "/") {
				if strings.Index(i, "*") != -1 {
					break
				}
				if syscall.OS == "windows" && len(i) == 2 && filepath.VolumeName(i) != "" {
					root = i + "/"
				} else {
					root = filepath.Join(root, i)
				}
			}
			if arg != root {
				if root == "" {
					root = "."
				} else {
					root += "/"
				}
			}

			filepath.Walk(root, g, nil)
		}
	}
}
