.PHONY: build test lint tidy fmt run migrate

SERVICE := account-svc
BINARY  := bin/$(SERVICE)
MAIN    := ./cmd/server

build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o $(BINARY) $(MAIN)

test:
	go test -race -cover ./...

test-integration:
	go test -tags=integration -race -v ./tests/integration/...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

fmt:
	gofmt -w -s .
	goimports -w .

run: build
	./$(BINARY)
