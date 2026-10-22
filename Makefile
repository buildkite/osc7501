# osc7501 Makefile

.PHONY: help test test-race bench fuzz lint check quality

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

test: ## Run all tests
	go test -v ./...

test-race: ## Run tests with race detector
	go test -race -v ./...

bench: ## Run benchmarks (compare runs with benchstat)
	go test -run '^$$' -bench . -benchmem -count 10 .

fuzz: ## Run each fuzz target for 20s
	go test -fuzz FuzzStripper -fuzztime 20s .
	go test -fuzz FuzzScanner -fuzztime 20s .

lint: ## Run golangci-lint
	golangci-lint run ./...

check: ## Run go vet and verify go.mod is tidy
	go vet ./...
	go mod tidy -diff

quality: check lint test ## Run all quality checks
