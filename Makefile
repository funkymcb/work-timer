.PHONY: build install test fmt vet check clean

build:
	go build -o bin/work ./cmd/work

install:
	go install ./cmd/work

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

check: fmt vet test

clean:
	rm -rf bin
