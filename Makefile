BINARY   := ly
MODULE   := github.com/Din-Studio/lingying-cli
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DATE     := $(shell date +%Y-%m-%d)
LDFLAGS  := -s -w -X $(MODULE)/cmd.Version=$(VERSION)
PREFIX   ?= /usr/local

.PHONY: all build vet install uninstall clean

all: vet build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

vet:
	go vet ./...

install: build
	install -m 755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "✅ $(BINARY) $(VERSION) installed to $(PREFIX)/bin/$(BINARY)"

uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)
	@echo "🗑 $(PREFIX)/bin/$(BINARY) removed"

clean:
	rm -f $(BINARY)
