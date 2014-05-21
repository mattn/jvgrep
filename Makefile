VERSION=$(shell ./jvgrep --help 2>&1 | grep ^Version | sed 's/Version //')

all : jvgrep

jvgrep: jvgrep.go
	go get -u -x code.google.com/p/mahonia
	go get -u -x github.com/daviddengcn/go-colortext
	go get -u -x github.com/mattn/jvgrep/mmap
	go build -x .

package: jvgrep.exe
	-rm -r jvgrep-win32-$(VERSION)
	-mkdir jvgrep-win32-$(VERSION)
	cp jvgrep.exe jvgrep-win32-$(VERSION)/.
	upx jvgrep-win32-$(VERSION)/jvgrep.exe
	tar cv jvgrep-win32-$(VERSION) | gzip > jvgrep-win32-$(VERSION).tar.gz
	-rm -r jvgrep-win32-$(VERSION)

upload:
	github-upload jvgrep-win32-$(VERSION).tar.gz mattn/jvgrep

dist: jvgrep
	git archive --format=tar --prefix=jvgrep-$(VERSION)/ HEAD | gzip > jvgrep-$(VERSION).tar.gz

clean :
	go clean
	-rm -f jvgrep
