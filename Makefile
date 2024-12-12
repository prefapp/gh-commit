BINARY_NAME=gh-commit

.PHONY: all build clean install uninstall

all: build

build:
	go build -o $(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)

install: build
	gh extension install .

uninstall: clean
	gh extension remove commit

reinstall: uninstall install
