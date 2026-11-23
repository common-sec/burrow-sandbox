# Sandbox image for pi (coding agent) (test preset test/presets/pi.preset), built by `make test-images`
# as localhost/burrow-test-pi:dev. Decision: docs/decisions/0009-agent-sandboxes.md.
# Pins: the base image by digest (Dependabot: docker); @earendil-works/pi-coding-agent 1.5.2 exactly, with integrity
# hashes for it and every dependency in package-lock.json (Dependabot: npm). All at least 7 days
# old when chosen. Licence: MIT (dependencies: MIT, Apache-2.0, BSD, ISC, BlueOak-1.0.0, 0BSD, Unlicense).
# The full (not slim) Node image already has git, curl and CA certificates, so nothing is
# installed with apt (Debian packages would not be hash-pinned).
FROM docker.io/library/node:26.13.1-bookworm@sha256:b7087bc4d973034b30854b378ba107f9fab622b47514cb7e2592e41b8bab8d49
WORKDIR /opt/agent
COPY package.json package-lock.json ./
# npm ci installs exactly the lockfile and checks every integrity hash.
RUN npm ci --omit=dev --no-audit --no-fund && npm cache clean --force
ENV PATH=/opt/agent/node_modules/.bin:$PATH
WORKDIR /work
# burrow starts the sandbox with its own command (sleep infinity) and runs the agent with
# `burrow shell NAME -- pi`.
ENTRYPOINT []
CMD ["pi"]
USER 1000:1000
