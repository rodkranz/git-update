.PHONY: test lint build fmt check ci clean

BINARY := git-update

test:
	go test -race -cover ./...

lint:
	golangci-lint run --timeout=5m ./...

build:
	go build -trimpath -o $(BINARY) .

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

check:
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"
	go vet ./...

ci: check test lint build

clean:
	rm -f $(BINARY) coverage.out
