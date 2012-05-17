VERSION=`./jvgrep -V`

jvgrep: jvgrep.go
	go build -x .

package: jvgrep.exe
	-rm -r jvgrep-win32-$(VERSION)
	-mkdir jvgrep-win32-$(VERSION)
	cp iconv.dll jvgrep-win32-$(VERSION)/jvgrep-iconv.dll
	cp jvgrep.exe jvgrep-win32-$(VERSION)/.
	upx jvgrep-win32-$(VERSION)/jvgrep.exe
	tar cv jvgrep-win32-$(VERSION) | gzip > jvgrep-win32-$(VERSION).tar.gz
	-rm -r jvgrep-win32-$(VERSION)

upload:
	github-upload jvgrep-win32-$(VERSION).tar.gz mattn/jvgrep
