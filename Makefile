.PHONY: build test install clean cross-compile lint

BINARY  := mysh
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./... -cover

test-verbose:
	go test ./... -cover -v

install:
	CGO_ENABLED=0 go install -ldflags "$(LDFLAGS)" .

clean:
	rm -f $(BINARY)
	rm -f mysh-*

lint:
	go vet ./...

# Cross-compile for all supported platforms
cross-compile: \
	build-linux-amd64 \
	build-linux-arm64 \
	build-darwin-amd64 \
	build-darwin-arm64 \
	build-windows-amd64 \
	build-windows-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-amd64 .

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-arm64 .

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-darwin-amd64 .

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-darwin-arm64 .

build-windows-amd64:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-windows-amd64.exe .

build-windows-arm64:
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-windows-arm64.exe .
