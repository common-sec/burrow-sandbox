# burrow-iron-proxy (PROXY=iron, decision 0013): iron-proxy, unchanged from its release image, and
# burrow's launcher (cmd/burrow-iron-proxy), on an empty image. Build: make images.
# iron-proxy 0.56.0 (2026-11-04, Apache-2.0) is pinned by the digest of its multi-arch index.
ARG GO_IMAGE=docker.io/library/golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280
ARG IRON_IMAGE=docker.io/ironsh/iron-proxy:0.56.0@sha256:727da0cc35a6d73ed2913a38d56c4b9b4476c075149492e4fbf75552e83361ff
FROM ${IRON_IMAGE} AS iron

FROM ${GO_IMAGE} AS build
ENV CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly" GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -ldflags "-s -w -buildid=" -o /out/burrow-iron-proxy ./cmd/burrow-iron-proxy

FROM scratch
# iron-proxy is a static Go binary. The CA bundle is for verifying upstreams once TLS is
# terminated for hosts with secrets (not used in sni-only mode).
COPY --from=iron /usr/local/bin/iron-proxy /iron-proxy
COPY --from=iron /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/burrow-iron-proxy /burrow-iron-proxy
# A numeric uid:gid needs no /etc/passwd; 65532 is the usual "nonroot" id (distroless).
USER 65532:65532
EXPOSE 3128
ENTRYPOINT ["/burrow-iron-proxy"]
