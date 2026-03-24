# goapplib Makefile
#
# Usage:
#   make setup    - Configure git hooks and local dev environment
#   make test     - Run all tests
#   make help     - Show available targets

.PHONY: setup test help

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

setup: ## Configure git hooks and local dev environment
	git config core.hooksPath .githooks
	@echo "Pre-push hook activated (.githooks/pre-push)"

test: ## Run all tests
	go test ./...
