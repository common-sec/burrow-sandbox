# burrow-forward (crates/burrow-forward, Rust, standard library only): static musl binary on an
# empty image. Build: `make images` (from the repository root). The build stage also runs the
# formatter, clippy and the tests, so the image cannot be built from code that fails them.
ARG RUST_IMAGE=docker.io/library/rust:1.99.1-alpine3.22@sha256:9a744dc47d1ac2a7d17fc3cedfc8b4928b1696dfcba3274ab25bfb40988a9ff0
FROM ${RUST_IMAGE} AS build
# clippy and rustfmt for the pinned toolchain, from rustup's checksummed channel manifest.
RUN rustup component add clippy rustfmt
WORKDIR /src
COPY crates/burrow-forward ./
# Reproducible: no build paths in the binary; no network for cargo (no dependencies).
ENV RUSTFLAGS="--remap-path-prefix=/src=." CARGO_NET_OFFLINE=true
RUN cargo fmt --check \
 && cargo clippy --locked --all-targets -- -D warnings \
 && cargo test --locked \
 && cargo build --locked --release \
 && install -D target/release/burrow-forward /out/burrow-forward

FROM scratch
COPY --from=build /out/burrow-forward /burrow-forward
# A numeric uid:gid needs no /etc/passwd; 65532 is the usual "nonroot" id (distroless).
USER 65532:65532
ENTRYPOINT ["/burrow-forward"]
