# Sandbox image for OpenAI Codex CLI (test preset test/presets/codex.preset), built by `make test-images`
# as localhost/burrow-test-codex:dev. Decision: docs/decisions/0009-agent-sandboxes.md.
# Pins: the base image by digest (Dependabot: docker); @openai/codex 0.169.0 exactly, with integrity
# hashes for it and every dependency in package-lock.json (Dependabot: npm). All at least 7 days
# old when chosen. Licence: Apache-2.0.
# The full (not slim) Node image already has git, curl and CA certificates, so nothing is
# installed with apt (Debian packages would not be hash-pinned).
FROM docker.io/library/node:24.22.3-bookworm@sha256:9d266751ca3d7fdbe651c878ffd5ad0a42fed49a18c283509267c97f025b7496
WORKDIR /opt/agent
COPY package.json package-lock.json ./
# npm ci installs exactly the lockfile and checks every integrity hash.
RUN npm ci --omit=dev --no-audit --no-fund && npm cache clean --force
ENV PATH=/opt/agent/node_modules/.bin:$PATH
WORKDIR /work
# burrow starts the sandbox with its own command (sleep infinity) and runs the agent with
# `burrow shell NAME -- codex`.
ENTRYPOINT []
CMD ["codex"]
USER 1000:1000
