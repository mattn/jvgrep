package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/mattn/go-iconv"
	"io/ioutil"
	"os"
	"path/filepath"
	"sre2.googlecode.com/hg/sre2"
	"runtime"
	"strings"
	"syscall"
)

const version = "0.5"

var encodings = []string{
	"iso-2022-jp-3",
	"iso-2022-jp",
	"euc-jisx0213",
	"euc-jp",
	"utf-8",
	"ucs-2",
	"euc-jp",
	"eucjp-ms",
	"cp932",
}

type grepper struct {
	glob    string
	pattern interface{}
	ere     sre2.Re
	oc      *iconv.Iconv
}

func (v *grepper) VisitDir(dir string, f *os.FileInfo) bool {
	if dir == "." || *recursive {
		return true
	}
	dirmask, _ := filepath.Split(v.glob)
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
	if v.ere != nil && v.ere.Match(path) {
		return
	}
	dirmask, filemask := filepath.Split(v.glob)
	dir, file := filepath.Split(path)

	dirmask = filepath.ToSlash(dirmask)
	if dirmask == "**/" || *recursive {
		dir = dirmask
	} else {
		dir = filepath.ToSlash(dir)
	}
	if *recursive && filemask == "" {
		filemask = "*"
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
			var match bool
			if re, ok := v.pattern.(sre2.Re); ok {
				if len(re.MatchIndex(string(t))) > 0 {
					match = true
				}
			} else if s, ok := v.pattern.(string); ok {
				if *ignorecase {
					if strings.Index(strings.ToLower(string(t)),
						strings.ToLower(s)) > -1 {
						match = true
					}
				} else {
					if strings.Index(string(t), s) > -1 {
						match = true
					}
				}
			}
			if (!*invert && !match) || (*invert && match) {
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
var exclude = flag.String("exclude", "", "exclude files: specify as regexp")
var fixed = flag.Bool("F", false, "fixed match")
var ignorecase = flag.Bool("i", false, "ignore case")
var infile = flag.String("f", "", "obtain pattern file")
var invert = flag.Bool("v", false, "invert match")
var list = flag.Bool("l", false, "listing files")
var recursive = flag.Bool("R", false, "recursive")
var ver = flag.Bool("V", false, "version")
var verbose = flag.Bool("S", false, "verbose")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: jvgrep [options] [pattern] [file...]\n")
		fmt.Fprintf(os.Stderr, "  Version %s", version)
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Supported Encodings:")
		for _, enc := range encodings {
			fmt.Fprintln(os.Stderr, "    "+enc)
		}
		os.Exit(-1)
	}
	flag.Parse()

	if *ver {
		fmt.Fprintf(os.Stdout, "%s\n", version)
		os.Exit(0)
	}
	if flag.NArg() == 0 {
		flag.Usage()
	}
	var err os.Error
	var errs *string
	var pattern interface{}

	instr := ""
	argindex := 0
	if len(*infile) > 0 {
		b, err := ioutil.ReadFile(*infile)
		if err != nil {
			println(err.String())
			os.Exit(-1)
		}
		instr = strings.TrimSpace(string(b))
	} else {
		instr = flag.Arg(0)
		argindex = 1
	}
	if *fixed {
		pattern = instr
	} else {
		if *ignorecase {
			instr = "(?i:" + instr + ")"
		}
		pattern, errs = sre2.Parse(instr)
		if errs != nil {
			println(*errs)
			os.Exit(-1)
		}
	}

	var ere sre2.Re
	if *exclude != "" {
		ere, errs = sre2.Parse(*exclude)
		if errs != nil {
			println(*errs)
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

	if flag.NArg() == 1 && argindex != 0 {
		g := &grepper{"", pattern, ere, oc}
		g.Grep(os.Stdin)
	} else {
		for _, arg := range flag.Args()[argindex:] {
			g := &grepper{filepath.ToSlash(arg), pattern, ere, oc}

			root := ""
			for n, i := range strings.Split(g.glob, "/") {
				if strings.Index(i, "*") != -1 {
					break
				}
				if n == 0 && i == "~" {
					if syscall.OS == "windows" {
						i = os.Getenv("USERPROFILE")
					} else {
						i = os.Getenv("HOME")
					}
				}

				root = filepath.Join(root, i)
				if n == 0 {
					if syscall.OS == "windows" && filepath.VolumeName(i) != "" {
						root = i + "/"
					} else if len(root) == 0 {
						root = "/"
					}
				}
			}
			if arg != root {
				if root == "" {
					root = "."
				} else {
					root += "/"
				}
			}
			root = filepath.Clean(root + "/")
			if *recursive && !strings.HasSuffix(g.glob, "/") {
				g.glob += "/"
			}

			filepath.Walk(root, g, nil)
		}
	}
}
