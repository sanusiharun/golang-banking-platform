FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates git tzdata

WORKDIR /app

COPY go.work go.work.sum* ./
COPY pkg/go.mod pkg/go.sum* ./pkg/
COPY services/auth-svc/go.mod services/auth-svc/go.sum* ./services/auth-svc/

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

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/auth-svc /auth-svc

USER nonroot:nonroot

EXPOSE 8082

ENTRYPOINT ["/auth-svc"]
