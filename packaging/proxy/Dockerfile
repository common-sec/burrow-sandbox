# burrow-proxy (test stand-in, see internal/proxy): static binary on an empty image.
# Build: mise run images (from the repository root).
ARG GO_IMAGE=docker.io/library/golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280
FROM ${GO_IMAGE} AS build
ENV CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly" GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -ldflags "-s -w -buildid=" -o /out/burrow-proxy ./cmd/burrow-proxy

FROM scratch
COPY --from=build /out/burrow-proxy /burrow-proxy
USER 65532:65532
EXPOSE 3128
ENTRYPOINT ["/burrow-proxy"]
