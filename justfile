# Developer shortcuts. `just check` is what CI runs.

set shell := ["bash", "-euo", "pipefail", "-c"]

# list recipes
default:
    @just --list

# gofmt -w on the whole module
fmt:
    gofmt -s -w .

# fail if anything is not gofmt-clean (no writes)
fmt-check:
    @test -z "$(gofmt -s -l .)" || { gofmt -s -l .; echo "run: just fmt"; exit 1; }

# go vet
vet:
    go vet ./...

# golangci-lint, same configuration as CI
lint:
    golangci-lint run ./...

# all tests with the race detector
test:
    go test -race ./...

# verbose tests, optionally filtered: just test-run 'TestDocuments'
test-run PATTERN='.':
    go test -v -run '{{PATTERN}}' ./...

# everything CI checks
check: fmt-check vet lint test

# coverage summary, writes coverage.out
cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out | tail -1

# refresh the schema snapshot from an introspection result of the API
schema INTROSPECTION:
    go run ./internal/schemasnap -in '{{INTROSPECTION}}'
