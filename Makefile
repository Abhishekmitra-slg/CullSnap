.PHONY: test run build package

APP_NAME := CullSnap
SRC := .

test:
	go test ./internal/...

run:
	wails dev

lint:
	go fmt ./...
	go vet ./...

build: lint
	go run github.com/wailsapp/wails/v2/cmd/wails@latest build

package-mac:
	go run github.com/wailsapp/wails/v2/cmd/wails@latest build -platform darwin/universal -clean
