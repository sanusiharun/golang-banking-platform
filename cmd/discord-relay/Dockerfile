FROM golang:1.23-alpine AS builder
WORKDIR /app
# Copy only what this self-contained module needs — no workspace, no shared pkg
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o discord-relay .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/discord-relay /discord-relay
EXPOSE 9094
ENTRYPOINT ["/discord-relay"]
