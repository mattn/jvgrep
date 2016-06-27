VERSION=$(shell ./jvgrep --help 2>&1 | grep ^Version | sed 's/Version //')

all : jvgrep

jvgrep: jvgrep.go
	go get -u -x github.com/mattn/go-colorable
	go get -u -x github.com/mattn/go-isatty
	go get -u -x github.com/mattn/jvgrep/mmap
	go build -x .

clean :
	go clean
	-rm -f jvgrep
