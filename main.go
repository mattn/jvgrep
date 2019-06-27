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

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/mattn/jvgrep/fastwalk"
	"github.com/mattn/jvgrep/mmap"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

const version = "5.7.0"

const (
	cMAGENTA = "\x1b[35;1m" // Color mazenta
	cCYAN    = "\x1b[36;1m" // Color cyan
	cGREEN   = "\x1b[32;1m" // Color green
	cRED     = "\x1b[31;1m" // Color red
	cRESET   = "\x1b[39;0m" // Color reset
)

var encodings = []string{
	"iso-2022-jp",
	"euc-jp",
	"utf-8",
	"sjis",
	"utf-16le",
	"utf-16be",
}

var (
	stdout = colorable.NewColorableStdout()
)

// GrepArg mean arguments to Grep.
type GrepArg struct {
	pattern interface{}
	input   interface{}
	size    int64
	single  bool
	atty    bool
	bom     []byte
}

const excludeDefaults = `(^|\/)\.git$|(^|\/)\.svn$|(^|\/)\.hg$|\.o$|\.obj$|\.a$|\.rlib$|\.exe~?$|(^|\/)tags$`

var (
	encs         string       // encodings
	exclude      string       // exclude pattern
	fixed        bool         // fixed search
	ignorecase   bool         // ignorecase
	ignorebinary bool         // ignorebinary
	infile       string       // input filename
	invert       bool         // invert search
	only         bool         // show only matched
	list         bool         // show the list matches
	number       bool         // show line number
	recursive    bool         // recursible search
	verbose      bool         // verbose output
	utf8out      bool         // output utf-8 strings
	perl         bool         // perl regexp syntax
	basic        bool         // basic regexp syntax
	oc           io.Writer    // output encoder
	color        string       // color operation
	cwd, _       = os.Getwd() // current directory
	zeroFile     bool         // write \0 after the filename
	zeroData     bool         // write \0 after the match
	allowTty     bool         // allow to search tty
	countMatch   = 0          // count of matches
	count        bool         // count of matches
	column       bool         // show column
	fullpath     = true       // show full path
	after        = 0          // show after lines
	before       = 0          // show before lines
	separator    = ":"        // column separator
)

var replbytes = []byte{0xef, 0xbf, 0xbd} // bytes representation of the replacement rune '\uFFFD'

func printLineZero(s string) {
	printStr(s + "\x00")
}

func printLineNorm(s string) {
	printStr(s + "\n")
}

var printLine = printLineNorm

func printStr(s string) {
	printBytes([]byte(s))
}

func printBytesUtf8(b []byte) {
	syscall.Write(syscall.Stdout, b)
}

func printBytesOutc(b []byte) {
	oc.Write(b)
}

func printBytesNorm(b []byte) {
	stdout.Write(b)
}

var printBytes = printBytesNorm

func matchedFile(f string) {
	if !fullpath {
		if fe, err := filepath.Rel(cwd, f); err == nil {
			f = fe
		}
	}
	printLine(f)
}

func matchedLine(f string, l, c int, m string, a *GrepArg) {
	lc := separator
	if l < 0 {
		lc = "-"
		l = -l
	}
	ls := fmt.Sprint(l)
	if column && c != -1 {
		ls += ":" + fmt.Sprint(c+1)
	}
	if !a.atty {
		if f != "" {
			if !fullpath {
				if fe, err := filepath.Rel(cwd, f); err == nil {
					f = fe
				}
			}
			if zeroFile {
				printStr(f + separator + ls + "\x00")
			} else {
				printStr(f + separator + ls + lc)
			}
		}
		printLine(m)
		return
	}
	if f != "" {
		if !fullpath {
			if fe, err := filepath.Rel(cwd, f); err == nil {
				f = fe
			}
		}
		if zeroFile {
			printStr(cMAGENTA + f + cRESET + "\x00" + cGREEN + ls + cCYAN + separator + cRESET)
		} else {
			printStr(cMAGENTA + f + cRESET + separator + cGREEN + ls + cCYAN + separator + cRESET)
		}
	}
	if re, ok := a.pattern.(*regexp.Regexp); ok {
		ill := re.FindAllStringIndex(m, -1)
		if len(ill) == 0 {
			printLine(m)
			return
		}
		for i, il := range ill {
			if i > 0 {
				printStr(m[ill[i-1][1]:il[0]] + cRED + m[il[0]:il[1]] + cRESET)
			} else {
				printStr(m[0:il[0]] + cRED + m[il[0]:il[1]] + cRESET)
			}
		}
		printLine(m[ill[len(ill)-1][1]:])
	} else if s, ok := a.pattern.(string); ok {
		l := len(s)
		for {
			i := strings.Index(m, s)
			if i < 0 {
				printLine(m)
				break
			}
			printStr(m[0:i] + cRED + m[i:i+l] + cRESET)
			m = m[i+l:]
		}
	}
}

func errorLine(s string) {
	os.Stderr.WriteString(s + "\n")
}

func maybeBinary(b []byte) bool {
	l := len(b)
	if l > 10000000 {
		l = 1024
	}
	if l > 1024 {
		l /= 2
	}
	for i := 0; i < l; i++ {
		if 0 < b[i] && b[i] < 0x9 {
			return true
		}
	}
	return false
}

func doGrep(path string, fb []byte, arg *GrepArg) bool {
	encs := encodings

	if ignorebinary {
		if maybeBinary(fb) {
			return false
		}
	}

	if len(fb) > 2 {
		if fb[0] == 0xfe && fb[1] == 0xff {
			arg.bom = fb[0:2]
			fb = fb[2:]
		} else if fb[0] == 0xff && fb[1] == 0xfe {
			arg.bom = fb[0:2]
			fb = fb[2:]
		} else if len(fb) > 3 && fb[0] == 0xef && fb[1] == 0xbb && fb[2] == 0xbf {
			arg.bom = fb[0:3]
			fb = fb[3:]
		}
	}
	if len(arg.bom) > 0 {
		if arg.bom[0] == 0xfe && arg.bom[1] == 0xff {
			encs = []string{"utf-16be"}
		} else if arg.bom[0] == 0xff && arg.bom[1] == 0xfe {
			encs = []string{"utf-16le"}
		} else if len(arg.bom) == 3 {
			encs = []string{""}
		}
	}

	re, _ := arg.pattern.(*regexp.Regexp)
	rs, _ := arg.pattern.(string)

	var okay bool
	var f []byte
	for _, enc := range encs {
		if verbose {
			println("trying("+enc+"):", path)
		}
		if len(arg.bom) == 2 && enc != "utf-16be" && enc != "utf-16le" {
			continue
		}

		did := false
		var t []byte
		var n, l, size, next, prev int

		f = fb
		if enc != "" {
			if len(arg.bom) > 0 || !maybeBinary(fb) {
				ee, _ := charset.Lookup(enc)
				if ee == nil {
					continue
				}
				var buf bytes.Buffer
				ic := transform.NewWriter(&buf, ee.NewDecoder())
				_, err := ic.Write(fb)
				if err != nil {
					if verbose {
						println(err.Error())
					}
					next = -1
					continue
				}
				lf := false
				if len(arg.bom) == 2 && len(fb)%2 != 0 {
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
				if bytes.Index(f, replbytes) > -1 {
					next = -1
					continue
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
				var matches [][]int
				ts := string(t)
				if re != nil {
					matches = re.FindAllStringIndex(ts, -1)
				} else {
					if ignorecase {
						ts = strings.ToLower(ts)
					}
					ti := 0
					tl := len(ts)
					matches = make([][]int, 0, 10)
					for ti != -1 && ti < tl-1 {
						ti = strings.Index(ts[ti:], rs)
						if ti != -1 {
							matches = append(matches, []int{ti, ti + tl})
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
					matchedFile(path)
					did = true
					break
				}
				for _, mm := range matches {
					countMatch++
					if count {
						continue
					}
					m := []byte(ts)[mm[0]:mm[1]]
					if arg.atty && maybeBinary(m) {
						errorLine(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					} else {
						if number {
							if utf8.Valid(m) {
								matchedLine(path, n, mm[0], string(m), arg)
							} else {
								errorLine(fmt.Sprintf("matched binary file: %s", path))
								did = true
								break
							}
						} else {
							if utf8.Valid(m) {
								matchedLine("", 0, mm[0], string(m), arg)
							} else {
								errorLine(fmt.Sprintf("matched binary file: %s", path))
								did = true
								break
							}
						}
					}
				}
			} else {
				var matches [][]int
				if re != nil {
					matches = re.FindAllIndex(t, 1)
				} else {
					if ignorecase {
						ti := strings.Index(strings.ToLower(string(t)), strings.ToLower(rs))
						if ti != -1 {
							matches = append(matches, []int{ti, ti + len(rs)})
						}
					} else {
						ti := strings.Index(string(t), rs)
						if strings.Index(string(t), rs) > -1 {
							matches = append(matches, []int{ti, ti + len(rs)})
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
					matchedFile(path)
					did = true
					break
				}
				countMatch++
				if count {
					did = true
					continue
				}
				matchedIndex := -1
				if match {
					matchedIndex = matches[0][0]
				}
				if arg.single && !number {
					if utf8.Valid(t) {
						matchedLine("", -1, matchedIndex, string(t), arg)
					} else {
						errorLine(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					}
				} else {
					if arg.atty && maybeBinary(t) {
						errorLine(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					} else if utf8.Valid(t) {
						if after <= 0 && before <= 0 {
							matchedLine(path, n, matchedIndex, string(t), arg)
						} else {
							if countMatch > 1 {
								os.Stdout.WriteString("---\n")
							}
							bprev, bnext := next-l-2, next-l-2
							lines := make([]string, 0, 10)
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
								matchedLine(path, i-n, matchedIndex, lines[i-1], arg)
							}
							matchedLine(path, n, matchedIndex, string(t), arg)
							lines = make([]string, 0, 10)
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
								matchedLine(path, -n-i-1, matchedIndex, lines[i], arg)
							}
						}
					} else {
						errorLine(fmt.Sprintf("matched binary file: %s", path))
						did = true
						break
					}
				}
			}
			did = true
		}
		if did {
			okay = true
		}
		if did || len(fb) == 0 {
			break
		}
	}
	return okay
}

// Grep do grep.
func Grep(arg *GrepArg) bool {
	n := false
	if in, ok := arg.input.(io.Reader); ok {
		stdin := bufio.NewReader(in)
		for {
			f, _, err := stdin.ReadLine()
			if doGrep("stdin", f, arg) {
				n = true
			}
			if err != nil {
				break
			}
		}
		return n
	}

	path, _ := arg.input.(string)
	if arg.size > 65536*4 {
		mf, err := mmap.Open(path)
		if err != nil {
			errorLine(err.Error() + ": " + path)
			return false
		}
		defer mf.Close()
		f := mf.Data()
		return doGrep(path, f, arg)
	}
	f, err := ioutil.ReadFile(path)
	if err != nil {
		errorLine(err.Error() + ": " + path)
		return false
	}
	return doGrep(path, f, arg)
}

func goGrep(ch chan *GrepArg, done chan bool) {
	n := 0
	for {
		arg := <-ch
		if arg == nil {
			break
		}
		if Grep(arg) {
			n++
		}
	}
	done <- n > 0
}

func showVersion() {
	fmt.Fprintf(os.Stdout, "%s\n", version)
	os.Exit(0)
}

func usage(simple bool) {
	fmt.Println("Usage: jvgrep [OPTION] PATTERN [FILE]...")
	if simple {
		fmt.Println("Try `jvgrep --help' for more information.")
	} else {
		fmt.Printf(`Version %s
Grep for Japanese vimmer. You can find text from files that written in
another Japanese encodings.

Regexp selection and interpretation:
  -F               : PATTERN is a set of newline-separated fixed strings
  -G               : PATTERN is a basic regular expression (BRE)
  -P               : PATTERN is a Perl regular expression (ERE)
  -f FILE          : obtain PATTERN from FILE
  -i               : ignore case
  -z, --null-data  : a data line ends in 0 byte, not newline
  --enc=ENCODINGS  : encodings of input files: comma separated
  --tty            : allow to search stdin even it is connected to a tty

Miscellaneous:
  -S               : verbose messages
  -V, --version    : print version information and exit

Output control:
  -8               : show result as utf-8 text
  -R               : search files recursively
  --exclude=REGEXP : exclude files: specify as REGEXP
                     (default: %s)
                     (specifying empty string won't exclude any files)
  --no-color       : do not print colors
  --color[=WHEN]   : always/never/auto
  -c               : count matches
  -C               : show column
  -r               : print relative path
  -I               : ignore binary files
  -l               : print only names of FILEs containing matches
  -n               : print line number with output lines
  -o               : show only the part of a line matching PATTERN
  -v               : select non-matching lines
  -Z, --null       : print 0 byte after FILE name
  --separator=CHAR : set column separator to CHAR (default: ":")

Context control:
  -B NUM           : print NUM lines of leading context
  -A NUM           : print NUM lines of trailing context

`, version, excludeDefaults)
		fmt.Println("Supported Encodings:")
		for _, enc := range encodings {
			if enc != "" {
				fmt.Println("    " + enc)
			}
		}
	}
	os.Exit(2)
}

func parseOptions() []string {
	var args []string

	argv := os.Args
	argc := len(argv)
	for n := 1; n < argc; n++ {
		if len(argv[n]) > 1 && argv[n][0] == '-' && argv[n][1] != '-' {
			switch argv[n][1] {
			case 'A':
				if len(argv[n]) > 2 {
					after, _ = strconv.Atoi(argv[n][2:])
					continue
				} else if n < argc-1 {
					after, _ = strconv.Atoi(argv[n+1])
					n++
					continue
				}
			case 'B':
				if len(argv[n]) > 2 {
					before, _ = strconv.Atoi(argv[n][2:])
					continue
				} else if n < argc-1 {
					before, _ = strconv.Atoi(argv[n+1])
					n++
					continue
				}
			case '8':
				utf8out = true
				printBytes = printBytesUtf8
			case 'F':
				fixed = true
			case 'R':
				recursive = true
			case 'S':
				verbose = true
			case 'c':
				count = true
			case 'C':
				column = true
			case 'r':
				fullpath = false
			case 'i':
				ignorecase = true
			case 'I':
				ignorebinary = true
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
				printLine = printLineZero
			case 'Z':
				zeroFile = true
			case 'V':
				showVersion()
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
			case name == "no-color":
				color = "never"
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
			case name == "tty":
				allowTty = true
			case name == "version":
				showVersion()
			case name == "help":
				usage(false)
			default:
				usage(true)
			}
		} else {
			args = append(args, argv[n])
		}
	}
	return args
}

func doMain() int {
	args := parseOptions()

	if len(args) == 0 {
		usage(true)
	}

	var err error
	var pattern interface{}
	if encs != "" {
		encodings = strings.Split(encs, ",")
	} else {
		encEnv := os.Getenv("JVGREP_ENCODINGS")
		if encEnv != "" {
			encodings = strings.Split(encEnv, ",")
		}
	}
	outEnc := os.Getenv("JVGREP_OUTPUT_ENCODING")
	if outEnc != "" {
		ee, _ := charset.Lookup(outEnc)
		if ee == nil {
			errorLine(fmt.Sprintf("unknown encoding: %s", outEnc))
			os.Exit(1)
		}
		oc = transform.NewWriter(stdout, ee.NewEncoder())
		if !utf8out {
			printBytes = printBytesOutc
		}
	}

	instr := ""
	argindex := 0
	if len(infile) > 0 {
		b, err := ioutil.ReadFile(infile)
		if err != nil {
			errorLine(err.Error())
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
			errorLine(err.Error())
			os.Exit(1)
		}
		rec, err := syntax.Compile(re)
		if err != nil {
			errorLine(err.Error())
			os.Exit(1)
		}
		instr = rec.String()
		if ignorecase {
			instr = "(?i:" + instr + ")"
		}
		if isLiteralRegexp(instr) {
			if verbose {
				println("pattern treated as literal:", instr)
			}
			pattern = instr
		} else {
			pattern, err = regexp.Compile(instr)
			if err != nil {
				errorLine(err.Error())
				os.Exit(1)
			}
		}
	} else {
		if ignorecase {
			instr = "(?i:" + instr + ")"
		}
		if isLiteralRegexp(instr) {
			if verbose {
				println("pattern treated as literal:", instr)
			}
			pattern = instr
		} else {
			pattern, err = regexp.Compile(instr)
			if err != nil {
				errorLine(err.Error())
				os.Exit(1)
			}
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
		errorLine(err.Error())
		os.Exit(1)
	}

	atty := false
	if color == "" {
		color = os.Getenv("JVGREP_COLOR")
	}
	if color == "" || color == "auto" {
		atty = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
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
				printStr(cRESET)
				os.Exit(0)
			}
		}()
	}

	if len(args) == 1 && argindex != 0 {
		if (isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())) && !allowTty {
			args = append(args, ".")
		} else {
			if Grep(&GrepArg{
				pattern: pattern,
				input:   os.Stdin,
				size:    -1,
				single:  true,
				atty:    atty,
			}) {
				return 1
			}
		}
	}

	globmask := ""

	ch := make(chan *GrepArg, 20)
	done := make(chan bool)
	go goGrep(ch, done)
	nargs := len(args[argindex:])
	for _, arg := range args[argindex:] {
		globmask = ""
		root := ""
		arg = strings.Trim(arg, `"`)
		fi, err := os.Stat(arg)
		if err == nil && fi.Mode().IsRegular() {
			// existing files: emit grep directly.
			ch <- &GrepArg{
				pattern: pattern,
				input:   arg,
				single:  false,
				atty:    atty,
			}
			continue
		} else if err == nil && fi.Mode().IsDir() {
			// existing directories: no need to prepare extra for glob.
		} else {
			// otherwise: prepare glob with expand path.
			root, globmask = prepareGlob(arg)
		}
		if root == "" {
			path, _ := filepath.Abs(arg)
			fi, err := os.Lstat(path)
			if err != nil {
				errorLine(fmt.Sprintf("jvgrep: %s: No such file or directory", arg))
				os.Exit(1)
			}
			if !fi.IsDir() {
				if fi.Size() == 0 {
					continue
				}
				if verbose {
					println("search:", path)
				}
				ch <- &GrepArg{
					pattern: pattern,
					input:   path,
					size:    fi.Size(),
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

		root = filepath.Clean(root)
		if root == "." {
			dirmask = "./" + dirmask
			filemask = "./" + filemask
		}

		dre := regexp.MustCompile("^" + dirmask)
		fre := regexp.MustCompile("^" + filemask + "$")

		if verbose {
			println("dirmask:", dirmask)
			println("filemask:", filemask)
			println("root:", root)
		}

		fastwalk.FastWalk(root, func(path string, mode os.FileMode) error {
			path = filepath.ToSlash(path)

			if ere != nil && ere.MatchString(path) {
				if mode.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if mode.IsDir() {
				if path == "." || recursive || len(path) <= len(root) || dre.MatchString(path+"/") {
					return nil
				}
				return filepath.SkipDir
			}

			if fre.MatchString(path) && mode.IsRegular() {
				if verbose {
					println("search:", path)
				}
				fi, err := os.Lstat(path)
				if err == nil {
					ch <- &GrepArg{
						pattern: pattern,
						input:   path,
						size:    fi.Size(),
						single:  false,
						atty:    atty,
					}
				}
			}
			return nil
		})
	}
	ch <- nil
	if count {
		fmt.Println(countMatch)
	}
	if <-done == false {
		return 1
	}
	return 0
}

var envre = regexp.MustCompile(`^(\$[a-zA-Z][a-zA-Z0-9_]+|\$\([a-zA-Z][a-zA-Z0-9_]+\))$`)

// prepareGlob prepares glob parameters with expanding `*`, `~` and environment
// variables in path.
func prepareGlob(arg string) (root, globmask string) {

	slashed := filepath.ToSlash(arg)
	volume := filepath.VolumeName(slashed)
	if volume != "" {
		slashed = slashed[len(volume):]
	}
	for n, i := range strings.Split(slashed, "/") {
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
	if volume != "" {
		root = filepath.Join(volume, root)
		globmask = filepath.Join(volume, globmask)
	}
	return root, globmask
}

// isLiteralRegexp checks regexp is a simple literal or not.
func isLiteralRegexp(expr string) bool {
	return regexp.QuoteMeta(expr) == expr
}

func main() {
	os.Exit(doMain())
}
