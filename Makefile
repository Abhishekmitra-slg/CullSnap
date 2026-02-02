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

package-mac:
	fyne package -os darwin -icon $(PWD)/internal/assets/icon.png -name CullSnap -release -src ./cmd/cullsnap
	zip -r CullSnap.app.zip CullSnap.app
