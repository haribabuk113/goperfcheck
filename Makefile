BINARY  := goperfcheck
MODULE  := github.com/haribabuk113/goperfcheck
CMD     := ./cmd/goperfcheck

.PHONY: build test lint vet fmt install clean cover

build:
	go build -o $(BINARY) $(CMD)

install:
	go install $(MODULE)/cmd/goperfcheck@latest

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .
	goimports -w .

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY) coverage.out

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
