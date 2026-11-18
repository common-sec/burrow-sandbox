# 0001: Repository layout for several products

**Status:** proposed (2026-11-18). **Asked by:** J ("a monorepo with sandbox, proxy, forwarder,
each one generating different packages... code within /src? or a folder per product?").

## Context

Three products today (the `burrow` CLI, the proxy, the forwarder), more later (shims, MCP server,
presets tooling). They share code (process handling, networking, config) and are released
differently: the CLI as binaries for six platforms, the proxy and forwarder as Linux container
images. All Go (DR-0006).

## Options

| Option | For | Against |
|---|---|---|
| **A. One module: `cmd/<product>/`, shared `internal/`, `packaging/<product>/`** | Go's standard layout; one `go.mod`, one lockfile (`go.sum`), one CI; shared code without versioning it; `internal/` cannot be imported from outside | all products share dependency versions; one tag versions everything |
| B. A folder (and module) per product, joined by `go.work` | independent versions and dependencies per product | shared code must be its own module and versioned; more go.mod/go.sum to pin and update; Dependabot entries per module |
| C. `src/` with everything below | familiar from other languages | not idiomatic in Go: import paths gain `/src/`, tools and readers expect `cmd/` and `internal/` |

## Decision

**A.** Products are `cmd/burrow`, `cmd/burrow-proxy`, `cmd/burrow-forward`. Shared packages are in
`internal/`. Each product's packaging (Dockerfile now, release config later) is in
`packaging/<product>/`. Move a product to its own module (option B) only when its dependencies or
release cycle really diverge, for example if the DR-0009 proxy integration brings heavy
dependencies that the CLI must not carry.

## Consequences

- Tags version the repository; products can still be released separately later with prefixed
  tags (`proxy/v0.3.0`) if they split.
- No `src/` folder.
