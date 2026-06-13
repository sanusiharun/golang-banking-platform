FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git tzdata

WORKDIR /app

COPY go.work go.work.sum* ./
COPY pkg/go.mod pkg/go.sum* ./pkg/

COPY services/account-svc/go.mod services/account-svc/go.sum* ./services/account-svc/
COPY services/auth-svc/go.mod services/auth-svc/go.sum* ./services/auth-svc/
COPY services/audit-svc/go.mod services/audit-svc/go.sum* ./services/audit-svc/

RUN go work sync

RUN cd services/auth-svc && go mod download

COPY pkg/ ./pkg/
COPY services/auth-svc/ ./services/auth-svc/

RUN cd services/auth-svc && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
      -trimpath \
      -o /bin/auth-svc \
      ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget

COPY --from=builder /bin/auth-svc /auth-svc

EXPOSE 8082

ENTRYPOINT ["/auth-svc"]
