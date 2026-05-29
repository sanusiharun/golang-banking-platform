FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o discord-relay ./cmd/discord-relay

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/discord-relay /discord-relay
EXPOSE 9094
ENTRYPOINT ["/discord-relay"]
