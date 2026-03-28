.PHONY: test run build package

APP_NAME := CullSnap
VERSION ?= dev
SRC := .

test:
	go test ./internal/...

run:
	wails dev

lint:
	go fmt ./...
	go vet ./...

build: lint
	go run github.com/wailsapp/wails/v2/cmd/wails@latest build -ldflags "-X main.version=$(VERSION)"
ifeq ($(shell uname),Darwin)
	@echo "Ad-hoc signing macOS app bundle..."
	codesign --force --deep --sign - build/bin/CullSnap.app
endif

package:
	./scripts/package.sh
