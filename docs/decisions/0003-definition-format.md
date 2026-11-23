# 0003: KEY=VALUE definitions for the prototype

**Status:** proposed, provisional (2026-11-18).

## Context

Environments and presets need a definition file. The research PoC used shell-sourced KEY=VALUE
files, which execute code. The prototype must not.

## Options

| Option | For | Against |
|---|---|---|
| **A. KEY=VALUE, parsed (no shell)** | no dependency; the PoC's files almost work as they are; easy to validate strictly | no lists or nesting; quoting rules are ours |
| B. TOML | lists and tables; mise uses it | a dependency (BurntSushi/toml or pelletier/go-toml) |
| C. YAML | common in container tooling (compose, kube) | a dependency; YAML's implicit typing surprises |
| D. Podman's own formats (Quadlet, kube YAML) | Podman-native (0002) | does not express presets, proxies or policy yet |

## Decision

**A for 0.1**, parsed strictly: unknown keys, duplicates and invalid values are errors; no
expansion or substitution. Revisit in 0.4 together with the presets repository (roadmap).
