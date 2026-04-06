package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf8"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/mattn/jvgrep/v5/mmap"
	ignore "github.com/sabhiram/go-gitignore"
	"github.com/saracen/walker"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

const (
	name     = "jvgrep"
	version  = "5.8.15"
	revision = "HEAD"
)

const (
	cMAGENTA = "\x1b[35;1m" // Color magenta
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

var stdout = colorable.NewColorableStdout()

// GrepArg mean arguments to Grep.
type GrepArg struct {
	pattern interface{}
	input   interface{}
	size    int64
	single  bool
	atty    bool
	bom     []byte
	ascii   bool
	buf     bytes.Buffer
}

func (a *GrepArg) writeStr(s string) {
	a.buf.WriteString(s)
}

func (a *GrepArg) writeLine(s string) {
	a.buf.WriteString(s)
	if zeroData {
		a.buf.WriteByte(0)
	} else {
		a.buf.WriteByte('\n')
	}
}

const excludeDefaults = `(^|\/)\.git$|(^|\/)\.svn$|(^|\/)\.hg$|` +
	`\.o$|\.obj$|\.a$|\.rlib$|\.so$|\.dll$|\.dylib$|\.lib$|\.class$|\.jar$|\.war$|\.pyc$|\.pyo$|\.wasm$|` +
	`\.[jJ][pP][gG]$|\.gif$|\.png$|\.bmp$|\.ico$|\.tiff?$|\.webp$|\.svg$|` +
	`\.gz$|\.zip$|\.tar$|\.bz2$|\.xz$|\.7z$|\.rar$|\.zst$|` +
	`\.pdf$|\.doc[x]?$|\.xls[x]?$|\.ppt[x]?$|` +
	`\.mp[34]$|\.avi$|\.mov$|\.wmv$|\.flv$|\.webm$|\.mkv$|\.wav$|\.flac$|\.ogg$|` +
	`\.ttf$|\.otf$|\.woff2?$|\.eot$|` +
	`\.[eE][xX][eE]~?$|(^|\/)tags$|` +
	`(^|\/)node_modules$|(^|\/)__pycache__$|(^|\/)site-packages$|` +
	`(^|\/)\.tox$|(^|\/)\.mypy_cache$|(^|\/)\.pytest_cache$`

// excludeExts is a fast extension-based lookup used when exclude is the default pattern.
var excludeExts = map[string]bool{
	".o": true, ".obj": true, ".a": true, ".rlib": true,
	".so": true, ".dll": true, ".dylib": true, ".lib": true,
	".class": true, ".jar": true, ".war": true, ".pyc": true, ".pyo": true, ".wasm": true,
	".jpg": true, ".jpeg": true, ".gif": true, ".png": true, ".bmp": true,
	".ico": true, ".tif": true, ".tiff": true, ".webp": true, ".svg": true,
	".gz": true, ".zip": true, ".tar": true, ".bz2": true, ".xz": true,
	".7z": true, ".rar": true, ".zst": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true,
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true, ".wmv": true,
	".flv": true, ".webm": true, ".mkv": true, ".wav": true, ".flac": true, ".ogg": true,
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
	".exe": true,
}

// excludeDirs is a fast directory name lookup used when exclude is the default pattern.
var excludeDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true,
	"node_modules": true, "__pycache__": true, "site-packages": true,
	".tox": true, ".mypy_cache": true, ".pytest_cache": true,
}

// isDefaultExcluded performs fast exclusion checks using maps instead of regex.
func isDefaultExcluded(path string, isDir bool) bool {
	base := filepath.Base(path)
	if isDir {
		return excludeDirs[base]
	}
	if base == "tags" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	return excludeExts[ext]
}

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
	recursive    bool         // recursive search
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
	countMatch   int64        // count of matches
	count        bool         // count of matches
	column       bool         // show column
	fullpath     = true       // show full path
	after        = 0          // show after lines
	before       = 0          // show before lines
	separator    = ":"        // column separator
	useGitIgnore bool         // respect .gitignore files
	skipHidden   bool         // skip hidden files/directories
)

type ignoreChecker struct {
	dir string
	gi  *ignore.GitIgnore
}

type gitIgnoreManager struct {
	matchers sync.Map // dir path -> *ignore.GitIgnore (or nil)
	gitRoots sync.Map // dir path -> bool
	chains   sync.Map // dir path -> []ignoreChecker (cached ancestor chain)
}

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

func matchedFile(f string, a *GrepArg) {
	if !fullpath {
		if fe, err := filepath.Rel(cwd, f); err == nil {
			f = fe
		}
	}
	a.writeLine(f)
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
				a.writeStr(f + separator + ls + "\x00")
			} else {
				a.writeStr(f + separator + ls + lc)
			}
		}
		a.writeLine(m)
		return
	}
	if f != "" {
		if !fullpath {
			if fe, err := filepath.Rel(cwd, f); err == nil {
				f = fe
			}
		}
		if zeroFile {
			a.writeStr(cMAGENTA + f + cRESET + "\x00" + cGREEN + ls + cCYAN + separator + cRESET)
		} else {
			a.writeStr(cMAGENTA + f + cRESET + separator + cGREEN + ls + cCYAN + separator + cRESET)
		}
	}
	if re, ok := a.pattern.(*regexp.Regexp); ok {
		ill := re.FindAllStringIndex(m, -1)
		if len(ill) == 0 {
			a.writeLine(m)
			return
		}
		for i, il := range ill {
			if i > 0 {
				a.writeStr(m[ill[i-1][1]:il[0]] + cRED + m[il[0]:il[1]] + cRESET)
			} else {
				a.writeStr(m[0:il[0]] + cRED + m[il[0]:il[1]] + cRESET)
			}
		}
		a.writeLine(m[ill[len(ill)-1][1]:])
	} else if s, ok := a.pattern.(string); ok {
		l := len(s)
		for {
			i := strings.Index(m, s)
			if i < 0 {
				a.writeLine(m)
				break
			}
			a.writeStr(m[0:i] + cRED + m[i:i+l] + cRESET)
			m = m[i+l:]
		}
	}
}

func errorLine(s string) {
	os.Stderr.WriteString(s + "\n")
}

func maybeBinary(b []byte) bool {
	// Check only the first 8KB, like ripgrep.
	l := len(b)
	if l > 8192 {
		l = 8192
	}
	for i := 0; i < l; i++ {
		if b[i] == 0x00 || (0 < b[i] && b[i] < 0x9) {
			return true
		}
	}
	return false
}

func (g *gitIgnoreManager) loadGitIgnore(dir string) *ignore.GitIgnore {
	if v, ok := g.matchers.Load(dir); ok {
		gi, _ := v.(*ignore.GitIgnore)
		return gi
	}
	gi, err := ignore.CompileIgnoreFile(dir + "/.gitignore")
	if err != nil {
		g.matchers.Store(dir, (*ignore.GitIgnore)(nil))
		return nil
	}
	g.matchers.Store(dir, gi)
	return gi
}

func (g *gitIgnoreManager) isGitRoot(dir string) bool {
	if v, ok := g.gitRoots.Load(dir); ok {
		return v.(bool)
	}
	_, err := os.Stat(dir + "/.git")
	isRoot := err == nil
	g.gitRoots.Store(dir, isRoot)
	return isRoot
}

func (g *gitIgnoreManager) getChain(dir string) []ignoreChecker {
	if v, ok := g.chains.Load(dir); ok {
		return v.([]ignoreChecker)
	}
	var dirs []string
	for d := dir; ; d = filepath.Dir(d) {
		dirs = append(dirs, d)
		if g.isGitRoot(d) || d == filepath.Dir(d) {
			break
		}
	}
	var chain []ignoreChecker
	for i := len(dirs) - 1; i >= 0; i-- {
		if gi := g.loadGitIgnore(dirs[i]); gi != nil {
			chain = append(chain, ignoreChecker{dir: dirs[i], gi: gi})
		}
	}
	g.chains.Store(dir, chain)
	return chain
}

func (g *gitIgnoreManager) isIgnored(absPath string, isDir bool) bool {
	dir := filepath.Dir(absPath)
	chain := g.getChain(dir)
	for _, c := range chain {
		rel := absPath[len(c.dir)+1:]
		if isDir {
			rel += "/"
		}
		if c.gi.MatchesPath(rel) {
			return true
		}
	}
	return false
}

func matchFixed(data, needle []byte) []int {
	idx := bytes.Index(data, needle)
	if idx < 0 {
		return nil
	}
	return []int{idx, idx + len(needle)}
}

func doGrepFixedUTF8(path string, fb []byte, arg *GrepArg, needle []byte) bool {
	if ignorebinary && maybeBinary(fb) {
		return false
	}

	if len(fb) >= 3 && fb[0] == 0xef && fb[1] == 0xbb && fb[2] == 0xbf {
		arg.bom = fb[:3]
		fb = fb[3:]
	} else {
		arg.bom = nil
	}

	if list && !invert {
		if bytes.Index(fb, needle) >= 0 {
			matchedFile(path, arg)
			return true
		}
		return false
	}

	var matched bool
	lineNo := 0
	start := 0
	size := len(fb)

	for start <= size {
		end := size
		if off := bytes.IndexByte(fb[start:], '\n'); off >= 0 {
			end = start + off
		}
		lineNo++
		line := fb[start:end]
		if l := len(line); l > 0 && line[l-1] == '\r' {
			line = line[:l-1]
		}

		var indexes [][]int
		if only {
			offset := 0
			for offset <= len(line)-len(needle) {
				idx := bytes.Index(line[offset:], needle)
				if idx < 0 {
					break
				}
				idx += offset
				indexes = append(indexes, []int{idx, idx + len(needle)})
				offset = idx + 1
			}
		} else if idx := matchFixed(line, needle); idx != nil {
			indexes = append(indexes, idx)
		}

		hasMatch := len(indexes) > 0
		if hasMatch == invert {
			if end == size {
				break
			}
			start = end + 1
			continue
		}
		if list {
			matchedFile(path, arg)
			return true
		}

		if only {
			for _, mm := range indexes {
				atomic.AddInt64(&countMatch, 1)
				if count {
					matched = true
					continue
				}
				part := line[mm[0]:mm[1]]
				if arg.atty && maybeBinary(part) {
					errorLine(fmt.Sprintf("matched binary file: %s", path))
					return true
				}
				if number {
					matchedLine(path, lineNo, mm[0], string(part), arg)
				} else {
					matchedLine("", 0, mm[0], string(part), arg)
				}
				matched = true
			}
		} else {
			atomic.AddInt64(&countMatch, 1)
			if count {
				matched = true
			} else {
				matchedIndex := -1
				if hasMatch {
					matchedIndex = indexes[0][0]
				}
				if arg.atty && maybeBinary(line) {
					errorLine(fmt.Sprintf("matched binary file: %s", path))
					return true
				}
				if arg.single && !number {
					matchedLine("", -1, matchedIndex, string(line), arg)
				} else {
					matchedLine(path, lineNo, matchedIndex, string(line), arg)
				}
				matched = true
			}
		}

		if end == size {
			break
		}
		start = end + 1
	}
	return matched
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
		if len(arg.bom) > 0 {
			if arg.bom[0] == 0xfe && arg.bom[1] == 0xff {
				encs = []string{"utf-16be"}
			} else if arg.bom[0] == 0xff && arg.bom[1] == 0xfe {
				encs = []string{"utf-16le"}
			} else if len(arg.bom) == 3 {
				encs = []string{""}
			}
		}
	}

	re, _ := arg.pattern.(*regexp.Regexp)
	rs, _ := arg.pattern.(string)

	if re == nil && rs != "" && !ignorecase && len(encs) == 1 && encs[0] == "utf-8" {
		return doGrepFixedUTF8(path, fb, arg, []byte(rs))
	}

	var okay bool
	var f []byte
	var istext bool
	for e, enc := range encs {
		if e > 0 && istext && arg.ascii && !strings.HasPrefix(enc, "utf-16") {
			continue
		}
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
			istext = true
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
			var matches [][]int
			if only {
				ts := string(t)
				if re != nil {
					matches = re.FindAllStringIndex(ts, -1)
				} else {
					if ignorecase {
						ts = strings.ToLower(ts)
					}
					ti := 0
					tl := len(ts)
					rl := len(rs)
					matches = make([][]int, 0, 10)
					for ti < tl {
						idx := strings.Index(ts[ti:], rs)
						if idx == -1 {
							break
						}
						matches = append(matches, []int{ti + idx, ti + idx + rl})
						ti += idx + 1
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
					matchedFile(path, arg)
					did = true
					break
				}
				for _, mm := range matches {
					atomic.AddInt64(&countMatch, 1)
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
						if ti > -1 {
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
					matchedFile(path, arg)
					did = true
					break
				}
				atomic.AddInt64(&countMatch, 1)
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
							if atomic.LoadInt64(&countMatch) > 1 {
								arg.buf.WriteString("---\n")
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
func flushArg(arg *GrepArg) {
	if arg.buf.Len() > 0 {
		printBytes(arg.buf.Bytes())
		arg.buf.Reset()
	}
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
			flushArg(arg)
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
		f := mf.Data()
		r := doGrep(path, f, arg)
		flushArg(arg)
		mf.Close()
		return r
	}
	f, err := os.ReadFile(path)
	if err != nil {
		errorLine(err.Error() + ": " + path)
		return false
	}
	r := doGrep(path, f, arg)
	flushArg(arg)
	return r
}

func goGrep(ch chan *GrepArg, done chan bool, mu *sync.Mutex) {
	n := 0
	for {
		arg := <-ch
		if arg == nil {
			break
		}
		path, ok := arg.input.(string)
		if !ok {
			mu.Lock()
			if Grep(arg) {
				n++
			}
			mu.Unlock()
			continue
		}
		// Read file outside lock for parallel I/O
		var data []byte
		var mf *mmap.Memfile
		var err error
		if arg.size > 65536*4 {
			mf, err = mmap.Open(path)
			if err != nil {
				mu.Lock()
				errorLine(err.Error() + ": " + path)
				mu.Unlock()
				continue
			}
			data = mf.Data()
		} else {
			data, err = os.ReadFile(path)
			if err != nil {
				mu.Lock()
				errorLine(err.Error() + ": " + path)
				mu.Unlock()
				continue
			}
		}
		// Grep outside lock for parallel matching
		matched := doGrep(path, data, arg)
		// Flush buffered output under lock
		mu.Lock()
		flushArg(arg)
		mu.Unlock()
		if mf != nil {
			mf.Close()
		}
		if matched {
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
  --gitignore      : respect .gitignore files
  --skip-hidden    : skip hidden files and directories
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
			case name == "gitignore":
				useGitIgnore = true
			case name == "skip-hidden":
				skipHidden = true
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
	for i := 0; i < len(encodings); i++ {
		encodings[i] = strings.ToLower(encodings[i])
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
		b, err := os.ReadFile(infile)
		if err != nil {
			errorLine(err.Error())
			os.Exit(1)
		}
		instr = strings.TrimSpace(string(b))
	} else {
		instr = args[0]
		argindex = 1
	}

	ascii := false
	if fixed {
		pattern = instr
		ascii = isASCII(instr)
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
			ascii = isASCII(instr)
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
			ascii = isASCII(instr)
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
	useDefaultExclude := exclude == ""
	if useDefaultExclude {
		exclude = excludeDefaults
	}
	var ere *regexp.Regexp
	if !useDefaultExclude {
		var err error
		ere, err = regexp.Compile(exclude)
		if err != nil {
			errorLine(err.Error())
			os.Exit(1)
		}
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
			for range sc {
				printStr(cRESET)
				os.Exit(0)
			}
		}()

		defer colorable.EnableColorsStdout(nil)()
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
				ascii:   ascii,
			}) {
				return 1
			}
		}
	}

	globmask := ""

	var mu sync.Mutex
	nworkers := runtime.GOMAXPROCS(0)
	if nworkers < 1 {
		nworkers = 1
	}
	ch := make(chan *GrepArg, nworkers*2)
	done := make(chan bool, nworkers)
	for i := 0; i < nworkers; i++ {
		go goGrep(ch, done, &mu)
	}
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
				ascii:   ascii,
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
					ascii:   ascii,
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

		absRoot, _ := filepath.Abs(root)
		absRoot = filepath.ToSlash(absRoot)
		var gim *gitIgnoreManager
		if useGitIgnore {
			gim = &gitIgnoreManager{}
		}

		isAbsRoot := filepath.IsAbs(root)
		walker.Walk(root, func(path string, mode os.FileInfo) error {
			path = filepath.ToSlash(path)

			base := path
			if i := strings.LastIndexByte(path, '/'); i >= 0 {
				base = path[i+1:]
			}

			isDir := mode.IsDir()

			if skipHidden && len(base) > 1 && base[0] == '.' {
				if isDir {
					return filepath.SkipDir
				}
				return nil
			}

			if gim != nil {
				if isDir && base == ".git" {
					if i := strings.LastIndexByte(path, '/'); i >= 0 {
						var parentAbs string
						if isAbsRoot {
							parentAbs = path[:i]
						} else {
							parentAbs = absRoot + "/" + path[:i]
						}
						gim.gitRoots.Store(parentAbs, true)
					}
					return filepath.SkipDir
				}

				var absPath string
				if isAbsRoot {
					absPath = path
				} else {
					absPath = absRoot + "/" + path
				}
				if gim.isIgnored(absPath, isDir) {
					if isDir {
						return filepath.SkipDir
					}
					return nil
				}
			}

			if useDefaultExclude {
				if isDir {
					if excludeDirs[base] {
						return filepath.SkipDir
					}
				} else {
					if base == "tags" {
						return nil
					}
					if dot := strings.LastIndexByte(base, '.'); dot >= 0 {
						if excludeExts[strings.ToLower(base[dot:])] {
							return nil
						}
					}
				}
			} else if ere != nil && ere.MatchString(path) {
				if isDir {
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

			if fre.MatchString(path) && mode.Mode().IsRegular() {
				if verbose {
					println("search:", path)
				}
				ch <- &GrepArg{
					pattern: pattern,
					input:   path,
					size:    mode.Size(),
					single:  false,
					atty:    atty,
					ascii:   ascii,
				}
			}
			return nil
		})
	}
	for i := 0; i < nworkers; i++ {
		ch <- nil
	}
	if count {
		fmt.Println(atomic.LoadInt64(&countMatch))
	}
	result := false
	for i := 0; i < nworkers; i++ {
		if <-done {
			result = true
		}
	}
	if !result {
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

func isASCII(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return r > 127
	}) == -1
}

// isLiteralRegexp checks regexp is a simple literal or not.
func isLiteralRegexp(expr string) bool {
	return regexp.QuoteMeta(expr) == expr
}

func main() {
	os.Exit(doMain())
}
