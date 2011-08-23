# Copyright 2011 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

include $(GOROOT)/src/Make.inc

VERSION=`./jvgrep -V`
TARG=jvgrep
GOFILES=\
	jvgrep.go\

include $(GOROOT)/src/Make.cmd

package: jvgrep.exe
	-rm -r jvgrep-win32-$(VERSION)
	-mkdir jvgrep-win32-$(VERSION)
	cp iconv.dll jvgrep-win32-$(VERSION)/.
	cp jvgrep.exe jvgrep-win32-$(VERSION)/.
	tar cv jvgrep-win32-$(VERSION) | gzip > jvgrep-win32-$(VERSION).tar.gz
