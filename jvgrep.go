package main

import (
	"bytes"
	"code.google.com/p/mahonia"
	"fmt"
	"github.com/mattn/jvgrep/mmap"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf8"
)

const version = "2.9"

var encodings = []string{
	"ascii",
	"iso-2022-jp",
	"utf-8",
	"euc-jp",
	"sjis",
	"utf-16",
	"",
}

type GrepArg struct {
	pattern interface{}
	input   interface{}
	single  bool
}

var encs string
var exclude string = `\.git|\.svn|\.hg|\.svn`
var fixed bool
var ignorecase bool
var infile string
var invert bool
var only bool
var list bool
var number bool
var recursive bool
var verbose bool
var utf8out bool
var perl bool
var basic bool

func printline(s string) bool {
	var err error
	if !utf8out {
		_, err = os.Stdout.WriteString(s + "\n")
	} else {
		_, err = syscall.Write(syscall.Stdout, []byte(s))
	}
	return err == nil
}

func errorline(s string) bool {
	_, err := os.Stderr.WriteString(s + "\n")
	return err == nil
}

func Grep(arg *GrepArg) {
	var f []byte
	var path = ""
	var ok bool
	var stdin *os.File
	var err error

	if path, ok = arg.input.(string); ok {
		mf, err := mmap.Open(path)
		if err != nil {
			errorline(err.Error())
			return
		}
		defer mf.Close()
		f = mf.Data()
	} else if stdin, ok = arg.input.(*os.File); ok {
		f, err = ioutil.ReadAll(stdin)
		if err != nil {
			errorline(err.Error())
			return
		}
		path = "stdin"
	}

	for _, enc := range encodings {
		if verbose {
			println("trying("+enc+"):", path)
		}

		did := false
		var t []byte
		var n, l, size, next, prev int

		if enc != "" {
			ic := mahonia.NewDecoder(enc)
			if ic == nil {
				continue
			}
			if enc == "utf-16" && len(f) > 2 {
				if f[0] == 0xfe && f[1] == 0xff {
					for nn := 0; nn < len(f); nn += 2 {
						f[nn], f[nn+1] = f[nn+1], f[nn]
					}
				}
			}
			ff, ok := ic.ConvertStringOK(string(f))
			if !ok {
				next = -1
				continue
			}
			f = []byte(ff)
		}
		size = len(f)
		if size == 0 {
			continue
		}

		for next != -1 {
			for {
				if next >= size {
					next = -1
					break
				}
				if f[next] == '\n' {
					break
				}
				next++
			}
			n++
			if next == -1 {
				t = f[prev:]
			} else {
				t = f[prev:next]
				prev = next + 1
				next++
			}

			l = len(t)
			if l > 0 && t[l-1] == '\r' {
				t = t[:l-1]
				l--
			}
			if l == 0 {
				continue
			}
			var match bool
			if only {
				var matches []string
				ts := string(t)
				if re, ok := arg.pattern.(*regexp.Regexp); ok {
					matches = re.FindAllString(ts, -1)
				} else if s, ok := arg.pattern.(string); ok {
					if ignorecase {
						ts = strings.ToLower(ts)
					}
					ti := 0
					tl := len(ts)
					for ti != -1 && ti < tl-1 {
						ti = strings.Index(ts[ti:], s)
						if ti != -1 {
							matches = append(matches, s)
							ti++
						}
					}
				}
				match = len(matches) > 0
				if (!invert && !match) || (invert && match) {
					continue
				}
				if verbose {
					println("found("+enc+"):", path)
				}
				if list {
					printline(path)
					did = true
					break
				}
				for _, m := range matches {
					if strings.IndexFunc(
						m, func(r rune) bool {
							return r < 0x9
						}) != -1 {
						errorline(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					} else {
						if number {
							if utf8.ValidString(m) {
								printline(fmt.Sprintf("%s:%d:%s", path, n, m))
							} else {
								errorline(fmt.Sprintf("matched binary file: %s", path))
								did = true
								break
							}
						} else {
							if utf8.ValidString(m) {
								printline(m)
							} else {
								errorline(fmt.Sprintf("matched binary file: %s", path))
								did = true
								break
							}
						}
					}
				}
			} else {
				if re, ok := arg.pattern.(*regexp.Regexp); ok {
					if len(re.FindAllIndex(t, 1)) > 0 {
						match = true
					}
				} else if s, ok := arg.pattern.(string); ok {
					if ignorecase {
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
				if (!invert && !match) || (invert && match) {
					continue
				}
				if verbose {
					println("found("+enc+"):", path)
				}
				if list {
					printline(path)
					did = true
					break
				}
				if arg.single && !number {
					if utf8.Valid(t) {
						printline(string(t))
					} else {
						errorline(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					}
				} else {
					if bytes.IndexFunc(
						t, func(r rune) bool {
							return r < 0x9
						}) != -1 {
						errorline(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					} else if utf8.Valid(t) {
						printline(fmt.Sprintf("%s:%d:%s", path, n, string(t)))
					} else {
						errorline(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					}
				}
			}
			did = true
		}
		runtime.GC()
		if did || next == -1 {
			break
		}
	}
}

func GoGrep(ch chan *GrepArg, done chan int) {
	for {
		arg := <-ch
		if arg == nil {
			break
		}
		Grep(arg)
	}
	done <- 1
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: jvgrep [OPTION] [PATTERN] [FILE...]
  Version %s
  -8               : show result as utf8 text
  -F               : PATTERN is a set of newline-separated fixed strings
  -G               : PATTERN is a basic regular expression (BRE)
  -P               : PATTERN is a Perl regular expression
  -R               : search files recursively
  -S               : verbose messages
  -V               : print version information and exit
  --enc encodings  : encodings: comma separated
  --exclude regexp : exclude files: specify as regexp
                     (default: \.git|\.svn|\.hg)
  -f file          : obtain pattern file
  -i               : ignore case(currently fixed only)
  -l               : print only names of FILEs containing matches
  -n               : print line number with output lines
  -o               : show only the part of a line matching PATTERN
  -v               : select non-matching lines

`, version)
	fmt.Fprintln(os.Stderr, "  Supported Encodings:")
	for _, enc := range encodings {
		if enc != "" {
			fmt.Fprintln(os.Stderr, "    "+enc)
		}
	}
	os.Exit(-1)
}

func main() {
	var args []string

	argv := os.Args
	argc := len(argv)
	for n := 1; n < argc; n++ {
		if len(argv[n]) > 1 && argv[n][0] == '-' && argv[n][1] != '-' {
			switch argv[n][1] {
			case '8':
				utf8out = true
			case 'F':
				fixed = true
			case 'R':
				recursive = true
			case 'S':
				verbose = true
			case 'i':
				ignorecase = true
			case 'l':
				list = true
			case 'n':
				number = true
			case 'P':
				perl = true
			case 'G':
				basic = true
			case 'v':
				invert = true
			case 'o':
				only = true
			case 'f':
				if n < argc-1 {
					infile = argv[n+1]
					n++
					continue
				}
			case 'V':
				fmt.Fprintf(os.Stdout, "%s\n", version)
				os.Exit(0)
			default:
				usage()
			}
			if len(argv[n]) > 2 {
				argv[n] = "-" + argv[n][2:]
				n--
			}
		} else if len(argv[n]) > 1 && argv[n][0] == '-' && argv[n][1] == '-' {
			if n == argc-1 {
				usage()
			}
			switch argv[n] {
			case "--enc":
				encs = argv[n+1]
			case "--exclude":
				exclude = argv[n+1]
			default:
				usage()
			}
			n++
		} else {
			args = append(args, argv[n])
		}
	}

	if len(args) == 0 {
		usage()
	}

	var err error
	var errs *string
	var pattern interface{}
	if encs != "" {
		encodings = strings.Split(encs, ",")
	} else {
		enc_env := os.Getenv("JVGREP_ENCODINGS")
		if enc_env != "" {
			encodings = strings.Split(enc_env, ",")
		}
	}

	instr := ""
	argindex := 0
	if len(infile) > 0 {
		b, err := ioutil.ReadFile(infile)
		if err != nil {
			errorline(err.Error())
			os.Exit(-1)
		}
		instr = strings.TrimSpace(string(b))
	} else {
		instr = args[0]
		argindex = 1
	}
	if fixed {
		pattern = instr
	} else if perl {
		re, err := syntax.Parse(instr, syntax.Perl)
		if err != nil {
			errorline(err.Error())
			os.Exit(-1)
		}
		rec, err := syntax.Compile(re)
		if err != nil {
			errorline(err.Error())
			os.Exit(-1)
		}
		instr = rec.String()
		if ignorecase {
			instr = "(?i:" + instr + ")"
		}
		pattern, err = regexp.Compile(instr)
		if err != nil {
			errorline(err.Error())
			os.Exit(-1)
		}
	} else {
		if ignorecase {
			instr = "(?i:" + instr + ")"
		}
		pattern, err = regexp.Compile(instr)
		if err != nil {
			errorline(err.Error())
			os.Exit(-1)
		}
	}

	var ere *regexp.Regexp
	if exclude != "" {
		ere, err = regexp.Compile(exclude)
		if errs != nil {
			errorline(err.Error())
			os.Exit(-1)
		}
	}
	if len(args) == 1 && argindex != 0 {
		Grep(&GrepArg{pattern, os.Stdin, true})
		return
	}

	envre := regexp.MustCompile(`^(\$[a-zA-Z][a-zA-Z0-9_]+|\$\([a-zA-Z][a-zA-Z0-9_]+\))$`)
	globmask := ""

	ch := make(chan *GrepArg)
	done := make(chan int)
	go GoGrep(ch, done)
	nargs := len(args[argindex:])
	for ai, arg := range args[argindex:] {
		globmask = ""
		root := ""
		arg = strings.Trim(arg, `"`)
		for n, i := range strings.Split(filepath.ToSlash(arg), "/") {
			if root == "" && strings.Index(i, "*") != -1 {
				if globmask == "" {
					root = "."
				} else {
					root = filepath.ToSlash(globmask)
				}
			}
			if n == 0 && i == "~" {
				if runtime.GOOS == "windows" {
					i = os.Getenv("USERPROFILE")
				} else {
					i = os.Getenv("HOME")
				}
			}
			if envre.MatchString(i) {
				i = strings.Trim(strings.Trim(os.Getenv(i[1:]), "()"), `"`)
			}

			globmask = filepath.Join(globmask, i)
			if n == 0 {
				if runtime.GOOS == "windows" && filepath.VolumeName(i) != "" {
					globmask = i + "/"
				} else if len(globmask) == 0 {
					globmask = "/"
				}
			}
		}
		if root == "" {
			path, _ := filepath.Abs(arg)
			if !recursive {
				if verbose {
					println("search:", path)
				}
				ch <- &GrepArg{pattern, path, ai == nargs-1}
				continue
			} else {
				root = path
				if fi, err := os.Stat(path); err == nil && fi.IsDir() {
					globmask = "**/*"
				} else {
					globmask = "**/" + globmask
				}
			}
		}
		if globmask == "" {
			globmask = "."
		}
		globmask = filepath.ToSlash(filepath.Clean(globmask))
		if recursive {
			if strings.Index(globmask, "/") > -1 {
				globmask += "/"
			} else {
				globmask = "**/" + globmask
			}
		}
		if runtime.GOOS == "windows" {
			// keep double backslask windows UNC.
			if len(arg) > 2 && (arg[0:2] == `\\` || arg[0:2] == `//`) {
				root = "/" + root
				globmask = "/" + globmask
			}
		}

		cc := []rune(globmask)
		dirmask := ""
		filemask := ""
		for i := 0; i < len(cc); i++ {
			if cc[i] == '*' {
				if i < len(cc)-2 && cc[i+1] == '*' && cc[i+2] == '/' {
					filemask += "(.*/)?"
					dirmask = filemask
					i += 2
				} else {
					filemask += "[^/]*"
				}
			} else {
				c := cc[i]
				if c == '/' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || 255 < c {
					filemask += string(c)
				} else {
					filemask += fmt.Sprintf("[\\x%x]", c)
				}
				if c == '/' && dirmask == "" && strings.Index(filemask, "*") != -1 {
					dirmask = filemask
				}
			}
		}
		if dirmask == "" {
			dirmask = filemask
		}
		if len(filemask) > 0 && filemask[len(filemask)-1] == '/' {
			if root == "" {
				root = filemask
			}
			filemask += "[^/]*"
		}
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			dirmask = "(?i:" + dirmask + ")"
			filemask = "(?i:" + filemask + ")"
		}
		dre := regexp.MustCompile("^" + dirmask)
		fre := regexp.MustCompile("^" + filemask + "$")

		root = filepath.Clean(root)

		if verbose {
			println("dirmask:", dirmask)
			println("filemask:", filemask)
			println("root:", root)
		}
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if info == nil {
				return err
			}

			path = filepath.ToSlash(path)

			if ere != nil && ere.MatchString(path) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if info.IsDir() {
				if path == "." || recursive || len(path) <= len(root) || dre.MatchString(path+"/") {
					return nil
				}
				return filepath.SkipDir
			}

			if fre.MatchString(path) {
				if verbose {
					println("search:", path)
				}
				ch <- &GrepArg{pattern, path, false}
			}
			return nil
		})
	}
	ch <- nil
	<-done
}
