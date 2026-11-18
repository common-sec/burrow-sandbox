# The single entry point for burrow, locally and in CI: CI runs these same targets (local first).
# mise provides the pinned tools (mise.toml + mise.lock); `mise exec` runs each command with them.
# On Windows, use WSL or Git Bash (docs/WINDOWS.md).
.DEFAULT_GOAL := help
MISE ?= mise
RUN = $(MISE) exec --
LOCK_PLATFORMS = linux-x64,linux-arm64,macos-x64,macos-arm64,windows-x64

.PHONY: help tools lock lock-check fmt lint test vuln build check scan scan-secrets scan-sast \
	scan-deps images e2e hooks clean

help: ## list the targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  make %-13s %s\n", $$1, $$2}' Makefile

tools: ## install the pinned tools from mise.lock (checksums verified)
	$(MISE) install

lock: ## update mise.lock after changing a version in mise.toml (review the diff)
	$(MISE) lock --platform $(LOCK_PLATFORMS)

lock-check: lock ## fail if mise.lock is missing or out of date (CI)
	@if [ -n "$$(git status --porcelain -- mise.lock)" ]; then \
		echo "::error::mise.lock is missing or out of date: run 'make lock' and commit it"; \
		cat mise.lock; exit 1; fi

fmt: ## format the Go code (gofumpt + goimports)
	$(RUN) golangci-lint fmt

lint: ## go vet and golangci-lint (gosec, staticcheck, errcheck, noctx, ...)
	$(RUN) go vet ./...
	$(RUN) golangci-lint run

test: ## unit tests, shuffled
	$(RUN) go test -count=1 -shuffle=on ./...

vuln: ## known vulnerabilities in reachable code (govulncheck)
	$(RUN) govulncheck ./...

build: ## static, reproducible binaries for every platform into dist/, with SHA256SUMS
	$(RUN) sh scripts/build.sh

check: lint test vuln build ## everything that must pass before a push

scan: scan-secrets scan-sast scan-deps ## TruffleHog, Semgrep, Trivy in pinned, locked-down containers

scan-secrets: ## TruffleHog: the tree and the whole git history, no network
	sh scripts/scan.sh secrets

scan-sast: ## Semgrep
	sh scripts/scan.sh sast

scan-deps: ## Trivy: dependencies, misconfigurations, secrets
	sh scripts/scan.sh deps

images: ## build the proxy and forwarder images (localhost/burrow-*:dev)
	sh scripts/images.sh

e2e: images ## end-to-end checks on real rootless Podman (docs/TESTING.md)
	$(RUN) sh scripts/e2e.sh

hooks: ## install a git pre-push hook that runs `make check scan`
	sh scripts/install-hooks.sh

clean: ## remove build output (dist/ and .cache/)
	rm -r -f dist .cache
