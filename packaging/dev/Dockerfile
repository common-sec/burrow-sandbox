# The developer checks (`make lint test vuln`), run inside pinned images so a machine needs only
# Podman: no Go, golangci-lint or mise on the host (decision 0010). The source goes in as the
# build context, never through a shared folder, so a Podman machine's share without file locking
# does not matter (docs/WINDOWS.md). Each check is a stage: `podman build --target <stage>`
# (scripts/dev.sh). Only the module download has network; the checks run with --network=none,
# except govulncheck, which reads vuln.go.dev.
# Pins: both images by tag and digest (Dependabot: docker, 7-day cooldown); Go modules by go.sum;
# govulncheck by the `tool` directive in go.mod.
FROM docker.io/library/golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS src
ENV CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly" GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal

FROM src AS vet
RUN --network=none go vet ./...

FROM src AS test
# The tests read the test presets and the example definitions.
COPY test/presets ./test/presets
COPY examples ./examples
RUN --network=none go test -count=1 -shuffle=on ./...

FROM src AS vuln
# Changes every day (scripts/dev.sh), so a cached result is never older than a day.
ARG VULN_DATE=unset
RUN echo "govulncheck, database of $VULN_DATE" && go tool govulncheck ./...

# golangci-lint's own image (built with Go 1.27). It reuses the modules downloaded above.
FROM docker.io/golangci/golangci-lint:v2.15.0-alpine@sha256:2ba875b990f1d8501ad4ec19be1ea0701e316ec3a4b764f7b129f62f01318bbf AS lint
ENV CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly" GOTOOLCHAIN=local
WORKDIR /src
COPY --from=src /go/pkg/mod /go/pkg/mod
COPY --from=src /src ./
COPY .golangci.yml ./
RUN --network=none golangci-lint run && golangci-lint fmt --diff
