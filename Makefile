.PHONY: test test-daemon test-gateway lint build clean hooks help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

test: test-daemon test-gateway ## Run all tests

test-daemon: ## Run daemon Go tests
	cd daemon && go test ./...

test-gateway: ## Run gateway TypeScript type check and tests
	cd gateway && npx tsc --noEmit
	cd gateway && npx bun test

lint: ## Run linters (go vet + tsc)
	cd daemon && go vet ./...
	cd gateway && npx tsc --noEmit

build: ## Build daemon binary
	cd daemon && go build -o ../bin/agentd ./cmd/agentd

clean: ## Remove build artifacts
	rm -rf bin/

hooks: ## Configure git hooks
	git config core.hooksPath .githooks
	@echo "Git hooks configured to use .githooks/"
