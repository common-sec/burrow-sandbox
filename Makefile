# The single entry point for burrow, locally and in CI: CI runs these same targets (local first).
# Every tool runs in a pinned container: the host needs only Podman and make, no Go, linters or
# mise (decision 0010). On Windows, use WSL or Git Bash (docs/WINDOWS.md).
.DEFAULT_GOAL := help

.PHONY: help fmt lint vet test vuln build forward check scan scan-secrets \
	scan-sast scan-deps scan-images scan-sandboxes images test-images sandbox-images e2e hooks clean

help: ## list the targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  make %-13s %s\n", $$1, $$2}' Makefile

fmt: ## format the Go code (gofumpt + goimports), in the pinned golangci-lint image
	sh scripts/dev.sh fmt

vet: ## go vet, in the pinned Go image, no network
	sh scripts/dev.sh vet

lint: vet ## go vet and golangci-lint (gosec, staticcheck, errcheck, noctx, ...), no network
	sh scripts/dev.sh lint

test: ## unit tests, shuffled, in the pinned Go image, no network
	sh scripts/dev.sh test

vuln: ## known vulnerabilities in reachable Go code (govulncheck, pinned in go.mod/go.sum)
	sh scripts/dev.sh vuln

build: ## static, reproducible binaries for every platform into dist/, with SHA256SUMS (in a container)
	sh scripts/build-in-container.sh

forward: ## the Rust forwarder: rustfmt, clippy, tests and build, inside its pinned build image
	sh scripts/images.sh forward

check: lint test vuln build forward ## everything that must pass before a push

scan: vuln scan-secrets scan-sast scan-deps scan-images ## govulncheck, TruffleHog, Semgrep, Trivy (code and images)

scan-secrets: ## TruffleHog: the tree and the whole git history, no network
	sh scripts/scan.sh secrets

scan-sast: ## Semgrep
	sh scripts/scan.sh sast

scan-deps: ## Trivy: dependencies, misconfigurations, secrets in the repository
	sh scripts/scan.sh deps

scan-images: images ## Trivy: vulnerabilities and secrets in the images we build
	sh scripts/scan.sh images

images: ## build the proxy (both engines) and forwarder images (localhost/burrow-*:dev)
	sh scripts/images.sh proxy forward

test-images: ## build the small test images the e2e needs (localhost/burrow-test-*:dev, test/images)
	sh scripts/images.sh test

sandbox-images: ## build the sandbox images of the test presets: ubuntu, kali, claude, codex, opencode, pi, hermes
	sh scripts/images.sh sandboxes

scan-sandboxes: ## Trivy on the sandbox images: secrets fail, vulnerabilities reported
	sh scripts/scan.sh sandboxes

e2e: build images test-images ## end-to-end checks on real rootless Podman (docs/TESTING.md)
	sh scripts/e2e.sh

hooks: ## install a git pre-push hook that runs `make check scan`
	sh scripts/install-hooks.sh

clean: ## remove build output (dist/ and .cache/)
	rm -r -f dist .cache
