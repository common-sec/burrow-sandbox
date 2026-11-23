# Builds the burrow binaries for every platform (dist/, with SHA256SUMS) inside a container, so a
# machine needs only Podman: no Go, no mise. The source goes in as the build context (copied into
# the Podman machine), not through the shared folder, so file locking on the share does not matter.
# Build: sh scripts/build-in-container.sh   (or the podman commands in docs/DEVELOPMENT.md)
ARG GO_IMAGE=docker.io/library/golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280
FROM ${GO_IMAGE} AS build
ENV CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly" GOTOOLCHAIN=local
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY scripts/build.sh ./scripts/build.sh
RUN BURROW_VERSION="$VERSION" sh scripts/build.sh

# Only the binaries: `podman create` + `podman cp` (or `podman build --output`) take them out.
FROM scratch
COPY --from=build /src/dist /dist
USER 65532:65532
