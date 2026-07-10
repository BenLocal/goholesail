# ---- build stage: compile the goholesail binary (the hub is a subcommand) ----
# goholesail is a Go project (module github.com/BenLocal/goholesail), so we build
# with the Go toolchain, not Rust.
FROM golang:1.26-alpine AS builder
WORKDIR /src

# Module proxy is overridable for build environments that can't reach the
# default (pass --build-arg GOPROXY=https://goproxy.cn,direct behind a firewall).
ARG GOPROXY
ENV GOPROXY=${GOPROXY:-https://proxy.golang.org,direct}

# Download modules first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Build a fully static binary (go-libp2p is pure Go, CGO not needed), so the
# runtime stage can be a minimal image.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
        -o /out/goholesail ./cmd/goholesail

# ---- runtime stage: minimal image with just the binary, non-root ----
FROM alpine:3.20
RUN adduser -D -H -u 10001 goholesail
COPY --from=builder /out/goholesail /usr/local/bin/goholesail

# The host role runs `goholesail host` via this wrapper (config from env / an
# env_file). The hub path ignores it (the hub compose overrides the command).
COPY deploy/goholesail-host /usr/local/bin/goholesail-host
USER goholesail

# Default hub listen port inside the container. docker-compose overrides the
# command to set the listen multiaddr / port and an optional stable --seed.
EXPOSE 4001
ENTRYPOINT ["goholesail"]
CMD ["hub", "--listen", "/ip4/0.0.0.0/tcp/4001"]
