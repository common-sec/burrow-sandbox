# 0008: The port forwarder is written in Rust

**Status:** proposed (2026-11-20). **Asked by:** J (second review of PR #1): "now that I'm
thinking, this probably would be safe in rust. rewrite it, please."

## Context

`burrow-forward` relays TCP from `127.0.0.1` on the host to one fixed address in a sandbox
(Podman cannot publish a port on a running container). It sits on a trust boundary: it is
reachable from the host and joined to the sandbox's internal network. It was a small Go program.

The organisation's [DR-0006](https://github.com/common-sec/secure-standard-platforms/blob/main/decisions/DR-0006-tooling-language.md)
says in-house tools are static Go binaries. It discarded Rust for the whole toolset (smaller
contributor pool) but names when to revisit: "if a component needs it (e.g. a sandbox helper with
tight syscall control)". The forwarder is such a sandbox helper; the exception is still recorded
here.

## Options

| Option | For | Against |
|---|---|---|
| **A. Rust, standard library only, `#![forbid(unsafe_code)]`, built in a pinned image** | memory safety without a garbage collector; no third-party crates to pin or audit; 443 kB scratch image; clippy and rustfmt as gates | a second language and toolchain; exception to org DR-0006 |
| B. Keep Go | one language; already tested | J asked for Rust; Go is memory-safe too, so the gain is mostly a smaller, simpler binary and no runtime |
| C. An existing forwarder (socat, a tiny Rust crate such as a TCP proxy binary) | no code of our own | socat is C with a large option surface (it can exec programs, open files); crates add dependencies and maintainer risk |

## Decision

**A.** `crates/burrow-forward`: one file, standard library only. It accepts only `IP:port` for
both addresses (a name could be resolved again to something else), dials only the fixed target
with a 5 s timeout, relays at most 256 connections at a time, closes both sides when either ends,
and logs JSON to stderr. The image (`packaging/forward/Dockerfile`) builds in
`rust:1.99.1-alpine3.22` pinned by digest (Rust 1.99.1, the newest past the 7-day cooldown on
2026-11-20; 1.100.0 was pushed 2026-11-17), runs `cargo fmt --check`, `cargo clippy -D warnings`,
`cargo test` and a `--locked` release build, and copies the static binary to `scratch`, user
`65532:65532`. Rust is MIT / Apache-2.0. `make forward` (part of `make check`) runs it.

## Exception to org DR-0006 (Go only)

| | |
|---|---|
| What | one in-house component in Rust |
| Owner | J |
| Reason | the forwarder is on a trust boundary; Rust with `forbid(unsafe_code)` and no crates gives a small, memory-safe binary with no runtime |
| Limits | standard library only; any crate needs a decision record; only this component |
| Expiry | review with the proxy decision (roadmap 0.3), which may replace the forwarder, or by 2027-05-20 |

## Why the others were discarded

- **B**: not what was asked, and no security property is lost by moving (the Go version was
  memory-safe too); the Rust version keeps the same behaviour and tests.
- **C**: more code and options than the job needs, or third-party crates for a 200-line program.

## Consequences and gaps

- Rust is not installed on the host: it lives only in the build image, as every other tool now does
  ([0010](0010-dev-tools-in-containers.md)). Dependabot watches the base image (docker) and `Cargo.lock` (cargo).
- Building needs network inside the build (rustup components for clippy and rustfmt). In networks
  that block it, the e2e was run with a local variant without those steps (TESTING.md).
- The reproducibility of the Rust build is not checked yet (*gap*, with releases).
