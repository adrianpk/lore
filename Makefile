GO := go
DEV_ROOT ?= _workspace
GO_BUILD_CACHE := $(CURDIR)/$(DEV_ROOT)/go-build-cache
GOLANGCI_LINT_CACHE := $(CURDIR)/$(DEV_ROOT)/golangci-lint-cache
COVERAGE_OUT ?= /tmp/lore-coverage.out

.PHONY: fmt test test-coverage lint lint-strict check verify docs-check check-go-style check-package-readmes clean-dev

fmt:
	@$(GO) fmt ./...

test:
	@$(GO) test ./...

test-coverage:
	@$(GO) test -coverprofile=$(COVERAGE_OUT) ./...
	@$(GO) tool cover -func=$(COVERAGE_OUT)

lint:
	@scripts/check-go-func-signatures.sh
	@scripts/check-package-readmes.sh
	@golangci-lint run ./...

lint-strict:
	@scripts/check-go-func-signatures.sh
	@scripts/check-package-readmes.sh
	@golangci-lint run --timeout=5m ./...

check-go-style:
	@scripts/check-go-func-signatures.sh

check-package-readmes:
	@scripts/check-package-readmes.sh

check: fmt check-go-style test

verify: lint-strict test

docs-check:
	@files="$$(git ls-files '*.md')"; test -n "$$files"; for document in $$files; do test -s "$$document"; done
	@if git grep -nE '[[:blank:]]+$$' -- '*.md'; then exit 1; fi

clean-dev:
	@rm -rf $(DEV_ROOT)
