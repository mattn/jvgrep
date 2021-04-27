jvgrep
======

`jvgrep` is grep for Japanese vimmer. You can find text from files that written in another Japanese encodings.

![](http://go-gyazo.appspot.com/8a66f5af5f60da99.png)

Download
--------

See [releases](https://github.com/mattn/jvgrep/releases)

Install
-------

To compile jvgrep, you must install golang.

> http://golang.org/

And type following

Go 1.16 or later

    # go install github.com/mattn/jvgrep/v5@latest

Go 1.15 or before

    # go get github.com/mattn/jvgrep/v5

#### Mac OS X

    # brew install jvgrep

Usage
-----

    -8               : show result as utf8 text
    -F               : PATTERN is a set of newline-separated fixed strings
    -G               : PATTERN is a basic regular expression (BRE)
    -P               : PATTERN is a Perl regular expression (ERE)
    -R               : search files recursively
    -S               : verbose messages
    -V               : print version information and exit
    --enc encodings  : encodings: comma separated
    --exclude regexp : exclude files: specify as regexp
                       (default: /\.git$|/\.svn$|/\.hg$|\.o$|\.obj$|\.a$|\.exe~?$|/tags$)
                       (specifying empty string won't exclude any files)
    --no-color       : do not print colors
    --color [=WHEN]  : always/never/auto
    -c               : count matches
    -r               : print relative path
    -f file          : obtain pattern file
    -i               : ignore case
    -l               : print only names of FILEs containing matches
    -I               : ignore binary files
    -n               : print line number with output lines
    -o               : show only the part of a line matching PATTERN
    -v               : select non-matching lines
    -z               : a data line ends in 0 byte, not newline
    -Z               : print 0 byte after FILE name
  
    Supported Encodings:
      ascii
      iso-2022-jp
      utf-8
      euc-jp
      sjis
      utf-16le
      utf-16be

for example,

    # jvgrep 表[現示] "**/*.txt"

`pattern` should be specify with regexp. `file` can be specify wildcard.
You can specify `pattern` with regular expression include multi-byte characters.
If you want to use own encodings for jvgrep, try to set environment variable $JVGREP_ENCODINGS to specify encodings separated with comma.
If you problem about output of jvgrep (ex: output of :grep command in vim), try to set $JVGREP_OUTPUT_ENCODING to specify encoding of output.

Supported Encodings
-------------------

* iso-2022-jp
* utf-8
* ucs-2
* euc-jp
* cp932
* utf-16 (support characters in utf-8)

Vim Enhancement
---------------

Add following to your vimrc

    set grepprg=jvgrep

Authors
-------

Yasuhiro Matsumoto

License
-------

under the MIT License: http://mattn.mit-license.org/2013
