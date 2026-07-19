SHELL := /bin/sh

.PHONY: fmt fmt-check vet test race coverage build lint check

fmt:
	gofmt -w .

fmt-check:
	test -z "$$(gofmt -l .)"

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

coverage:
	go test -coverprofile=coverage.out ./...
	./scripts/check-coverage.sh coverage.out 80

build:
	go build ./...

lint:
	golangci-lint run ./...

check: fmt-check vet test race coverage build lint
