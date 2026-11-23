# Sandbox image for Claude Code (test preset test/presets/claude.preset), built by `make test-images`
# as localhost/burrow-test-claude:dev. Decision: docs/decisions/0009-agent-sandboxes.md.
# Pins: the base image by digest (Dependabot: docker); @anthropic-ai/claude-code 2.1.325 exactly, with integrity
# hashes for it and every dependency in package-lock.json (Dependabot: npm). All at least 7 days
# old when chosen. Licence: proprietary, Anthropic's terms (the package's LICENSE.md); built locally, not redistributed.
# The full (not slim) Node image already has git, curl and CA certificates, so nothing is
# installed with apt (Debian packages would not be hash-pinned).
FROM docker.io/library/node:26.13.1-bookworm@sha256:b7087bc4d973034b30854b378ba107f9fab622b47514cb7e2592e41b8bab8d49
WORKDIR /opt/agent
COPY package.json package-lock.json ./
# npm ci installs exactly the lockfile and checks every integrity hash.
# Its postinstall copies the native binary for this platform from an optional dependency.
RUN npm ci --omit=dev --no-audit --no-fund && npm cache clean --force
# Updates come from a new pinned image, never from the agent itself.
ENV PATH=/opt/agent/node_modules/.bin:$PATH DISABLE_AUTOUPDATER=1
WORKDIR /work
# burrow starts the sandbox with its own command (sleep infinity) and runs the agent with
# `burrow shell NAME -- claude`.
ENTRYPOINT []
CMD ["claude"]
USER 1000:1000
