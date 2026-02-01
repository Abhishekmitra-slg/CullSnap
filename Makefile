.PHONY: test run build package

APP_NAME := CullSnap
SRC := ./cmd/cullsnap

test:
	go test ./internal/...

run:
	go run $(SRC)

lint:
	go fmt ./...
	go vet ./...

build: lint
	go build -ldflags "-s -w" -o bin/$(APP_NAME) $(SRC)

package:
	fyne package -os darwin -icon internal/assets/icon.png
