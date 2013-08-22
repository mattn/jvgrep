jvgrep
======

`jvgrep` is grep for japanese vimmer. you can find text from files that writen in another japanese encodings.

Install
-------

To compile jvgrep, you must install golang.

> http://golang.org/

And type following

    # go get github.com/mattn/jvgrep

Usage
-----

    Usage: jvgrep [options] [pattern] [file...]
      Version 0.7
      -F=false: fixed match
      -R=false: recursive
      -S=false: verbose
      -V=false: version
      -enc="": encodings: comma separated
      -exclude="": exclude files: specify as regexp
      -f="": obtain pattern file
      -i=false: ignore case(currently fixed only)
      -l=false: listing files
      -v=false: invert match
    
      Supported Encodings:
        iso-2022-jp
        euc-jp
        utf-8
        euc-jp
        cp932
        utf-16

for example,

    # jvgrep 表[現示] "**/*.txt"

`pattern` should be specify with regexp. `file` can be specify wildcard.
You can specify `pattern` with regular expression include multi-byte characters..
If you want to use own encodings for jvgrep, try to set environment variable $JVGREP_ENCODINGS to specify encodings separated with comma.
If you problem about output of jvgrep (ex: output of :grep command in vim), try to set $JVGREP_OUTPUT_ENCODING to specify encoding of output.

Supported Encodings
-------------------

* iso-2022-jp
* euc-jp
* utf-8
* ucs-2
* euc-jp
* cp932
* utf-16 (support characters in utf-8)

Vim Enhancement
---------------

Add following to your vimrc

    set grepprg=jvgrep

