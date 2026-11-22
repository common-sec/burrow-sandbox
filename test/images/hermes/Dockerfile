# Sandbox image for Hermes Agent by Nous Research (test preset test/presets/hermes.preset), built
# by `make test-images` as localhost/burrow-test-hermes:dev. Decision: docs/decisions/0009-agent-sandboxes.md.
# Pins: the base image by digest (Dependabot: docker); hermes-agent 0.19.0 from PyPI and every
# dependency by version and SHA-256 in requirements.txt (generated from requirements.in with
# `uv pip compile --generate-hashes --exclude-newer <7 days ago>`; Dependabot: pip). Licence: MIT.
# The full Python image already has git, curl and CA certificates (no apt).
FROM docker.io/library/python:3.13.18-bookworm@sha256:cc55e4ded9c95f6ce2133b3d096db43f214d280c2a8af65d83e246c8fa7d168c
COPY requirements.txt /opt/agent/requirements.txt
# --require-hashes: pip refuses anything not listed with its hash.
RUN python -m venv /opt/agent/venv \
    && /opt/agent/venv/bin/pip install --no-cache-dir --require-hashes --no-deps -r /opt/agent/requirements.txt
ENV PATH=/opt/agent/venv/bin:$PATH
WORKDIR /work
ENTRYPOINT []
CMD ["hermes"]
USER 1000:1000
