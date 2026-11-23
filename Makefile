.PHONY: test vet fmt lint check

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint:
	golangci-lint run ./...

check: vet lint test
	@test -z "$$(gofmt -s -l .)"
