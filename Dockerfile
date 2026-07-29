# syntax=docker/dockerfile:1

# ---- build ----------------------------------------------------------------
# Pinned to the builder's native architecture: the target architecture is
# reached through Go cross-compilation (GOARCH), never QEMU emulation, so a
# linux/arm64 image costs the same to build as a linux/amd64 one.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

ARG TARGETARCH
# Escape hatch for memory-constrained builders: --build-arg GOFLAGS=-p=2 caps
# parallel package compiles. Empty by default so the shipped Dockerfile does
# not slow down builds on machines that have the headroom.
ARG GOFLAGS=""

# modernc.org/sqlite is pure Go, so there is no C toolchain to install and the
# binary links statically. GOWORK=off keeps go.work — which lists the
# Windows-only client module — out of the resolution; server/go.mod's
# `replace cyberstalk.me/shared => ../shared` is all that is needed.
ENV CGO_ENABLED=0 GOOS=linux GOWORK=off GOFLAGS=${GOFLAGS}

WORKDIR /src
# Module files first so the dependency layer survives ordinary source edits.
COPY shared/go.mod shared/
COPY server/go.mod server/go.sum server/
RUN cd server && go mod download

COPY shared/ shared/
COPY server/ server/
RUN cd server && GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' -o /out/cyberstalk-server ./cmd/server

# Built here, not in the runtime stage, so the runtime stage stays RUN-free.
RUN mkdir -p /out/data

# ---- runtime --------------------------------------------------------------
# No RUN instructions below this line. Nothing has to execute on the target
# architecture, which is what lets the multi-arch build skip QEMU entirely.
# Adding a RUN here means release.yml must also set up docker/setup-qemu-action.
#
# alpine rather than distroless on purpose: deploying this means running
# `docker compose exec app cyberstalk-server register-device ...`, wget-based
# health checks, and the occasional look inside /data. A few MB buys that.
FROM alpine:3.24

COPY --from=build /out/cyberstalk-server /usr/local/bin/cyberstalk-server
COPY --from=build --chown=65532:65532 /out/data /data

ENV ADDR=:8080 SQLITE_PATH=/data/cyberstalk.db
EXPOSE 8080
VOLUME ["/data"]
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/cyberstalk-server"]
