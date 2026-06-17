.PHONY: run build test lint tidy

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -o bin/notification-svc ./cmd/server

test:
	go test ./...

test-integration:
	go test -tags=integration ./tests/integration/...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy
