# Sandbox image for Hermes Agent by Nous Research (test preset test/presets/hermes.preset), built
# by `make test-images` as localhost/burrow-test-hermes:dev. Decision: docs/decisions/0009-agent-sandboxes.md.
# Pins: the base image by digest (Dependabot: docker); hermes-agent 0.19.0 from PyPI and every
# dependency by version and SHA-256 in requirements.txt (generated from requirements.in with
# `uv pip compile --generate-hashes --exclude-newer <7 days ago>`; Dependabot: pip). Licence: MIT.
# The full Python image already has git, curl and CA certificates (no apt).
FROM docker.io/library/python:3.15.2-bookworm@sha256:6f060cab421d9509c6c6a6a7762c651baf03c3f1d0145befe16b688762b7fe81
COPY requirements.txt /opt/agent/requirements.txt
# --require-hashes: pip refuses anything not listed with its hash.
RUN python -m venv /opt/agent/venv \
    && /opt/agent/venv/bin/pip install --no-cache-dir --require-hashes --no-deps -r /opt/agent/requirements.txt
ENV PATH=/opt/agent/venv/bin:$PATH
WORKDIR /work
ENTRYPOINT []
CMD ["hermes"]
USER 1000:1000
