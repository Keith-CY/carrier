.PHONY: test test-daemon test-gateway test-openclaw-installer test-start-systemd coverage-gate lint build clean hooks help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

test: test-daemon test-gateway test-openclaw-installer test-start-systemd ## Run all tests

test-daemon: ## Run daemon Go tests
	cd daemon && go test ./...

test-gateway: ## Run gateway Go tests
	cd gateway && go test ./...

test-openclaw-installer: ## Run OpenClaw installer checksum regression test
	cd daemon/internal/catalog && bash ./openclaw-installer_test.sh

test-start-systemd: ## Run start.sh systemd target regression test
	bash scripts/start_systemd_test.sh

coverage-gate: ## Run multi-module coverage gate
	bash scripts/coverage-gate.sh

lint: ## Run linters (go vet)
	cd daemon && go vet ./...

build: ## Build daemon binary
	cd daemon && go build -o ../bin/agentd ./cmd/agentd

clean: ## Remove build artifacts
	rm -rf bin/

hooks: ## Configure git hooks
	git config core.hooksPath .githooks
	@echo "Git hooks configured to use .githooks/"
