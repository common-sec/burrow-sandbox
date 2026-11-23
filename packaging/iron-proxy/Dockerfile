# burrow-iron-proxy (PROXY=iron, decision 0013): iron-proxy and burrow's launcher
# (cmd/burrow-iron-proxy), static binaries on an empty image. Build: make images.
#
# iron-proxy is built here from its released Go module, v0.56.0 (2026-11-04, Apache-2.0), not
# copied from its release image: packaging/iron-proxy/build/go.sum pins the module and every
# dependency, checked against the Go checksum database, and raises the dependencies whose fixes
# the release lacks (go-jose 4.1.6, x/net 0.62.0, grpc 1.83.2: see decision 0013).
ARG GO_IMAGE=docker.io/library/golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280
FROM ${GO_IMAGE} AS build
ENV CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly" GOTOOLCHAIN=local
WORKDIR /iron
COPY packaging/iron-proxy/build/go.mod packaging/iron-proxy/build/go.sum ./
RUN go mod download && go mod verify \
 && go build -ldflags "-s -w -buildid= -X github.com/ironsh/iron-proxy/internal/version.Version=0.56.0" \
      -o /out/iron-proxy github.com/ironsh/iron-proxy/cmd/iron-proxy
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -ldflags "-s -w -buildid=" -o /out/burrow-iron-proxy ./cmd/burrow-iron-proxy

FROM scratch
# The CA bundle is for verifying upstreams once TLS is terminated for hosts with secrets (not used
# in sni-only mode).
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/iron-proxy /iron-proxy
COPY --from=build /out/burrow-iron-proxy /burrow-iron-proxy
# A numeric uid:gid needs no /etc/passwd; 65532 is the usual "nonroot" id (distroless).
USER 65532:65532
EXPOSE 3128
ENTRYPOINT ["/burrow-iron-proxy"]
