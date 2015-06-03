package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/daviddengcn/go-colortext"
	"github.com/mattn/go-isatty"
	"github.com/mattn/jvgrep/mmap"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

const version = "4.1"

var encodings = []string{
	"iso-2022-jp",
	"euc-jp",
	"sjis",
	"utf-8",
	"utf-16le",
	"utf-16be",
}

type GrepArg struct {
	pattern interface{}
	input   interface{}
	single  bool
	atty    bool
	bom     []byte
}

const excludeDefaults = `\.git$|\.svn$|\.hg$|\.o$|\.obj$|\.exe$`

var (
	encs       string       // encodings
	exclude    string       // exclude pattern
	fixed      bool         // fixed search
	ignorecase bool         // ignorecase
	infile     string       // input filename
	invert     bool         // invert search
	only       bool         // show only matched
	list       bool         // show the list matches
	number     bool         // show line number
	recursive  bool         // recursible search
	verbose    bool         // verbose output
	utf8out    bool         // output utf-8 strings
	perl       bool         // perl regexp syntax
	basic      bool         // basic regexp syntax
	oc         io.Writer    // output encoder
	color      string       // color operation
	cwd, _     = os.Getwd() // current directory
	zeroFile   bool         // write \0 after the filename
	zeroData   bool         // write \0 after the match
	countMatch = 0          // count of matches
	count      bool         // count of matches
	fullpath   = true       // show full path
	after      = 0          // show after lines
	before     = 0          // show before lines
	separator  = ":"        // column separator
)

func matchedfile(f string) {
	if !fullpath {
		if fe, err := filepath.Rel(cwd, f); err == nil {
			f = fe
		}
	}
	if zeroFile {
		printstr(f)
		os.Stdout.Write([]byte{0})
	} else {
		printline(f)
	}
}

func matchedline(f string, l int, m string, a *GrepArg) {
	lc := separator
	if l < 0 {
		lc = "-"
		l = -l
	}
	if !a.atty {
		if f != "" {
			if !fullpath {
				if fe, err := filepath.Rel(cwd, f); err == nil {
					f = fe
				}
			}
			if zeroFile {
				printstr(fmt.Sprintf("%s%s%d\x00", f, separator, l))
			} else {
				printstr(fmt.Sprintf("%s%s%d%s", f, separator, l, lc))
			}
		}
		printline(m)
		return
	}
	if f != "" {
		if !fullpath {
			if fe, err := filepath.Rel(cwd, f); err == nil {
				f = fe
			}
		}
		ct.ChangeColor(ct.Magenta, true, ct.None, false)
		printstr(f)
		ct.ChangeColor(ct.Cyan, true, ct.None, false)
		if zeroFile {
			os.Stdout.Write([]byte{0})
		} else {
			fmt.Print(separator)
		}
		ct.ChangeColor(ct.Green, true, ct.None, false)
		fmt.Print(l)
		ct.ChangeColor(ct.Cyan, true, ct.None, false)
		fmt.Print(separator)
		ct.ResetColor()
	}
	if re, ok := a.pattern.(*regexp.Regexp); ok {
		ill := re.FindAllStringIndex(m, -1)
		if len(ill) == 0 {
			printline(m)
			return
		}
		for i, il := range ill {
			if i > 0 {
				printstr(m[ill[i-1][1]:il[0]])
			} else {
				printstr(m[0:il[0]])
			}
			ct.ChangeColor(ct.Red, true, ct.None, false)
			printstr(m[il[0]:il[1]])
			ct.ResetColor()
		}
		printline(m[ill[len(ill)-1][1]:])
	} else if s, ok := a.pattern.(string); ok {
		l := len(s)
		for {
			i := strings.Index(m, s)
			if i < 0 {
				printline(m)
				break
			}
			printstr(m[0:i])
			ct.ChangeColor(ct.Red, true, ct.None, false)
			printstr(m[i : i+l])
			ct.ResetColor()
			m = m[i+l:]
		}
	}
}

func printline(s string) {
	printstr(s + "\n")
	if zeroData {
		os.Stdout.Write([]byte{0})
	}
}

func printstr(s string) {
	if utf8out {
		syscall.Write(syscall.Stdout, []byte(s))
	} else if oc != nil {
		oc.Write([]byte(s))
	} else {
		os.Stdout.WriteString(s)
	}
}

func errorline(s string) {
	os.Stderr.WriteString(s + "\n")
}

func doGrep(path string, f []byte, arg *GrepArg) {
	encs := encodings

	if len(f) > 2 {
		if f[0] == 0xfe && f[1] == 0xff {
			arg.bom = f[0:2]
			f = f[2:]
		} else if f[0] == 0xff && f[1] == 0xfe {
			arg.bom = f[0:2]
			f = f[2:]
		}
	}
	if len(arg.bom) > 0 {
		if arg.bom[0] == 0xfe && arg.bom[1] == 0xff {
			encs = []string{"utf-16be"}
		} else if arg.bom[0] == 0xff && arg.bom[1] == 0xfe {
			encs = []string{"utf-16le"}
		}
	}

	for _, enc := range encs {
		if verbose {
			println("trying("+enc+"):", path)
		}
		if len(arg.bom) > 0 && enc != "utf-16be" && enc != "utf-16le" {
			continue
		}

		did := false
		var t []byte
		var n, l, size, next, prev int

		if enc != "" {
			if len(arg.bom) > 0 || bytes.IndexFunc(f, func(r rune) bool { return 0 < r && r < 0x9 }) == -1 {
				ee, _ := charset.Lookup(enc)
				if ee == nil {
					continue
				}
				var buf bytes.Buffer
				ic := transform.NewWriter(&buf, ee.NewDecoder())
				_, err := ic.Write(f)
				if err != nil {
					next = -1
					continue
				}
				lf := false
				if len(arg.bom) > 0 && len(f)%2 != 0 {
					ic.Write([]byte{0})
					lf = true
				}
				err = ic.Close()
				if err != nil {
					if verbose {
						println(err.Error())
					}
					next = -1
					continue
				}
				f = buf.Bytes()
				if lf {
					f = f[:len(f)-1]
				}
			}
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
				// skip if not match without invert, or match with invert.
				if match == invert {
					continue
				}
				if verbose {
					println("found("+enc+"):", path)
				}
				if list {
					matchedfile(path)
					did = true
					break
				}
				for _, m := range matches {
					countMatch++
					if count {
						continue
					}
					if strings.IndexFunc(
						m, func(r rune) bool {
							return 0 < r && r < 0x9
						}) != -1 {
						errorline(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					} else {
						if number {
							if utf8.ValidString(m) {
								matchedline(path, n, m, arg)
							} else {
								errorline(fmt.Sprintf("matched binary file: %s", path))
								did = true
								break
							}
						} else {
							if utf8.ValidString(m) {
								matchedline("", 0, m, arg)
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
				// skip if not match without invert, or match with invert.
				if match == invert {
					continue
				}
				if verbose {
					println("found("+enc+"):", path)
				}
				if list {
					matchedfile(path)
					did = true
					break
				}
				countMatch++
				if count {
					did = true
					continue
				}
				if arg.single && !number {
					if utf8.Valid(t) {
						matchedline("", -1, string(t), arg)
					} else {
						errorline(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					}
				} else {
					if bytes.IndexFunc(
						t, func(r rune) bool {
							return 0 < r && r < 0x9
						}) != -1 {
						errorline(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					} else if utf8.Valid(t) {
						if after <= 0 && before <= 0 {
							matchedline(path, n, string(t), arg)
						} else {
							if countMatch > 1 {
								os.Stdout.WriteString("---\n")
							}
							bprev, bnext := next-l-2, next-l-2
							lines := make([]string, 0)
							for i := 0; i < before && bprev > 0; i++ {
								for {
									if bprev == 0 || f[bprev-1] == '\n' {
										lines = append(lines, string(f[bprev:bnext]))
										bnext = bprev - 1
										bprev--
										break
									}
									bprev--
								}
							}
							for i := len(lines); i > 0; i-- {
								matchedline(path, i-n, lines[i-1], arg)
							}
							matchedline(path, n, string(t), arg)
							lines = make([]string, 0)
							aprev, anext := next, next
							for i := 0; i < after && anext >= 0 && anext < size; i++ {
								for {
									if anext == size || f[anext] == '\n' {
										lines = append(lines, string(f[aprev:anext]))
										aprev = anext + 1
										anext++
										break
									}
									anext++
								}
							}
							for i := 0; i < len(lines); i++ {
								matchedline(path, -n-i-1, lines[i], arg)
							}
						}
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

func Grep(arg *GrepArg) {
	var f []byte
	var path = ""
	var ok bool
	var stdin *bufio.Reader

	if path, ok = arg.input.(string); ok {
		if fi, err := os.Stat(path); err == nil && fi.Size() == 0 {
			return
		}
		mf, err := mmap.Open(path)
		if err != nil {
			errorline(err.Error() + ": " + path)
			return
		}
		defer mf.Close()
		f = mf.Data()
		doGrep(path, f, arg)
	} else if in, ok := arg.input.(io.Reader); ok {
		stdin = bufio.NewReader(in)
		for {
			f, _, err := stdin.ReadLine()
			doGrep("stdin", f, arg)
			if err != nil {
				break
			}
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

func usage(simple bool) {
	fmt.Fprintln(os.Stderr, "Usage: jvgrep [OPTION] [PATTERN] [FILE]...")
	if simple {
		fmt.Fprintln(os.Stderr, "Try `jvgrep --help' for more information.")
	} else {
		fmt.Fprintf(os.Stderr, `Version %s
Regexp selection and interpretation:
  -F               : PATTERN is a set of newline-separated fixed strings
  -G               : PATTERN is a basic regular expression (BRE)
  -P               : PATTERN is a Perl regular expression (ERE)

Miscellaneous:
  -S               : verbose messages
  -V               : print version information and exit

Output control:
  -8               : show result as utf8 text
  -R               : search files recursively
  --enc encodings  : encodings: comma separated
  --exclude regexp : exclude files: specify as regexp
                     (default: %s)
  --color [=WHEN]  : always/never/auto
  -c               : count matches
  -r               : print relative path
  -f file          : obtain pattern file
  -i               : ignore case(currently fixed only)
  -l               : print only names of FILEs containing matches
  -n               : print line number with output lines
  -o               : show only the part of a line matching PATTERN
  -v               : select non-matching lines
  -z               : a data line ends in 0 byte, not newline
  -Z               : print 0 byte after FILE name

Context control:
  -B               : print NUM lines of leading context
  -A               : print NUM lines of trailing context
`, version, excludeDefaults)
		fmt.Fprintln(os.Stderr, "  Supported Encodings:")
		for _, enc := range encodings {
			if enc != "" {
				fmt.Fprintln(os.Stderr, "    "+enc)
			}
		}
	}
	os.Exit(2)
}

func main() {
	var args []string

	argv := os.Args
	argc := len(argv)
	for n := 1; n < argc; n++ {
		if len(argv[n]) > 1 && argv[n][0] == '-' && argv[n][1] != '-' {
			switch argv[n][1] {
			case 'A':
				if n < argc-1 {
					after, _ = strconv.Atoi(argv[n+1])
					n++
					continue
				}
			case 'B':
				if n < argc-1 {
					before, _ = strconv.Atoi(argv[n+1])
					n++
					continue
				}
			case '8':
				utf8out = true
			case 'F':
				fixed = true
			case 'R':
				recursive = true
			case 'S':
				verbose = true
			case 'c':
				count = true
			case 'r':
				fullpath = false
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
			case 'z':
				zeroData = true
			case 'Z':
				zeroFile = true
			case 'V':
				fmt.Fprintf(os.Stdout, "%s\n", version)
				os.Exit(0)
			default:
				usage(true)
			}
			if len(argv[n]) > 2 {
				argv[n] = "-" + argv[n][2:]
				n--
			}
		} else if len(argv[n]) > 1 && argv[n][0] == '-' && argv[n][1] == '-' {
			name := argv[n][2:]
			switch {
			case strings.HasPrefix(name, "enc="):
				encs = name[4:]
			case name == "enc" && n < argc-1:
				encs = argv[n+1]
				n++
			case strings.HasPrefix(name, "exclude="):
				exclude = name[8:]
			case name == "exclude" && n < argc-1:
				exclude = argv[n+1]
				n++
			case strings.HasPrefix(name, "color="):
				color = name[6:]
			case name == "color" && n < argc-1:
				color = argv[n+1]
				n++
			case strings.HasPrefix(name, "separator="):
				separator = name[10:]
			case name == "separator":
				separator = argv[n+1]
				n++
			case name == "null":
				zeroFile = true
			case name == "null-data":
				zeroData = true
			case name == "help":
				usage(false)
			default:
				usage(true)
			}
		} else {
			args = append(args, argv[n])
		}
	}

	if len(args) == 0 {
		usage(true)
	}

	var err error
	var pattern interface{}
	if encs != "" {
		encodings = strings.Split(encs, ",")
	} else {
		enc_env := os.Getenv("JVGREP_ENCODINGS")
		if enc_env != "" {
			encodings = strings.Split(enc_env, ",")
		}
	}
	out_enc := os.Getenv("JVGREP_OUTPUT_ENCODING")
	if out_enc != "" {
		ee, _ := charset.Lookup(out_enc)
		if ee == nil {
			errorline(fmt.Sprintf("unknown encoding: %s", out_enc))
			os.Exit(1)
		}
		oc = transform.NewWriter(os.Stdout, ee.NewEncoder())
	}

	instr := ""
	argindex := 0
	if len(infile) > 0 {
		b, err := ioutil.ReadFile(infile)
		if err != nil {
			errorline(err.Error())
			os.Exit(1)
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
			os.Exit(1)
		}
		rec, err := syntax.Compile(re)
		if err != nil {
			errorline(err.Error())
			os.Exit(1)
		}
		instr = rec.String()
		if ignorecase {
			instr = "(?i:" + instr + ")"
		}
		pattern, err = regexp.Compile(instr)
		if err != nil {
			errorline(err.Error())
			os.Exit(1)
		}
	} else {
		if ignorecase {
			instr = "(?i:" + instr + ")"
		}
		pattern, err = regexp.Compile(instr)
		if err != nil {
			errorline(err.Error())
			os.Exit(1)
		}
	}

	if exclude == "" {
		exclude = os.Getenv("JVGREP_EXCLUDE")
	}
	if exclude == "" {
		exclude = excludeDefaults
	}
	ere, err := regexp.Compile(exclude)
	if err != nil {
		errorline(err.Error())
		os.Exit(1)
	}

	atty := false
	if color == "" {
		color = os.Getenv("JVGREP_COLOR")
	}
	if color == "" || color == "auto" {
		atty = isatty.IsTerminal(os.Stdout.Fd())
	} else if color == "always" {
		atty = true
	} else if color == "never" {
		atty = false
	} else {
		usage(true)
	}

	if atty {
		sc := make(chan os.Signal, 10)
		signal.Notify(sc, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		go func() {
			for _ = range sc {
				ct.ResetColor()
				os.Exit(0)
			}
		}()
	}

	if len(args) == 1 && argindex != 0 {
		Grep(&GrepArg{
			pattern: pattern,
			input:   os.Stdin,
			single:  true,
			atty:    atty,
		})
		return
	}

	envre := regexp.MustCompile(`^(\$[a-zA-Z][a-zA-Z0-9_]+|\$\([a-zA-Z][a-zA-Z0-9_]+\))$`)
	globmask := ""

	ch := make(chan *GrepArg, 10)
	done := make(chan int)
	go GoGrep(ch, done)
	nargs := len(args[argindex:])
	for _, arg := range args[argindex:] {
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
			fi, err := os.Stat(path)
			if err != nil {
				errorline(fmt.Sprintf("jvgrep: %s: No such file or directory", arg))
				os.Exit(1)
			}
			if !recursive && !fi.IsDir() {
				if verbose {
					println("search:", path)
				}
				ch <- &GrepArg{
					pattern: pattern,
					input:   path,
					single:  nargs == 1,
					atty:    atty,
				}
				continue
			} else {
				root = path
				if fi.IsDir() {
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

			if fre.MatchString(path) && info.Mode().IsRegular() {
				if verbose {
					println("search:", path)
				}
				ch <- &GrepArg{
					pattern: pattern,
					input:   path,
					single:  false,
					atty:    atty,
				}
			}
			return nil
		})
	}
	ch <- nil
	if count {
		fmt.Println(countMatch)
	}
	<-done
}
