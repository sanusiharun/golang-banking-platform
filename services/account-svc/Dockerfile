# ──────────────────────────────────────────────────────────────────────────────
# Stage 1: Build
# Uses Go 1.23 Alpine for a small build image.
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# Install certificates for HTTPS calls during go mod download.
RUN apk add --no-cache ca-certificates git tzdata

WORKDIR /app

# Copy workspace descriptor first so Docker cache is invalidated only when
# module requirements change.
COPY go.work go.work.sum* ./

# Copy module files for both workspace members.
COPY pkg/go.mod pkg/go.sum* ./pkg/
COPY services/account-svc/go.mod services/account-svc/go.sum* ./services/account-svc/

# Download dependencies (cached layer).
RUN cd services/account-svc && go mod download

# Copy full source.
COPY pkg/ ./pkg/
COPY services/account-svc/ ./services/account-svc/

# Build the binary with size optimisations.
# CGO_ENABLED=0 produces a fully static binary compatible with distroless.
RUN cd services/account-svc && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
      -trimpath \
      -o /bin/account-svc \
      ./cmd/server

# ──────────────────────────────────────────────────────────────────────────────
# Stage 2: Runtime — distroless for minimal attack surface.
# ──────────────────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

# Copy timezone data and CA certificates from builder.
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the statically-linked binary.
COPY --from=builder /bin/account-svc /account-svc

# Run as non-root user (UID 65532 in distroless/nonroot).
USER nonroot:nonroot

# HTTP API port and Prometheus metrics port.
EXPOSE 8080 9090

ENTRYPOINT ["/account-svc"]
