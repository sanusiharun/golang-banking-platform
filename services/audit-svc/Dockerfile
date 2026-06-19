# ──────────────────────────────────────────────────────────────────────────────
# Stage 1: Build
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git tzdata

WORKDIR /app

# Copy workspace descriptor so go work sync sees all modules.
COPY go.work go.work.sum* ./
COPY pkg/go.mod pkg/go.sum* ./pkg/

# Copy every service go.mod — required for go work sync.
COPY services/auth-svc/go.mod services/auth-svc/go.sum* ./services/auth-svc/
COPY services/account-svc/go.mod services/account-svc/go.sum* ./services/account-svc/
COPY services/audit-svc/go.mod services/audit-svc/go.sum* ./services/audit-svc/
COPY services/notification-svc/go.mod services/notification-svc/go.sum* ./services/notification-svc/
COPY services/payment-svc/go.mod services/payment-svc/go.sum* ./services/payment-svc/

RUN go work sync

RUN cd services/audit-svc && go mod download

# Copy full source.
COPY pkg/ ./pkg/
COPY services/audit-svc/ ./services/audit-svc/

RUN cd services/audit-svc && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
      -trimpath \
      -o /bin/audit-svc \
      ./cmd/server

# ──────────────────────────────────────────────────────────────────────────────
# Stage 2: Runtime
# ──────────────────────────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget

COPY --from=builder /bin/audit-svc /audit-svc

EXPOSE 8083

ENTRYPOINT ["/audit-svc"]
