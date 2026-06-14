package audit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// PublisherConfig configures NewPublisher.
type PublisherConfig struct {
	// NATSURL is the NATS server URL (e.g. "nats://platform-nats:4222").
	// Empty string → NoopPublisher is returned immediately.
	NATSURL string

	// ServiceName labels the NATS connection and log messages.
	ServiceName string

	// ConnectWait is how long to wait for the TCP handshake after nats.Connect
	// returns (RetryOnFailedConnect is non-blocking). Default: 5s.
	ConnectWait time.Duration
}

// NewPublisher creates an audit Publisher for the given service.
// The publisher is wrapped with Async so callers never block on Publish.
//
// Falls back to NoopPublisher (audit silently discarded) when:
//   - NATSURL is empty
//   - NATS is unreachable within ConnectWait
//   - JetStream / stream setup fails
//
// The returned *nats.Conn must be drained on service shutdown (nc.Drain()).
// nc is nil when NoopPublisher is returned — safe to ignore.
func NewPublisher(ctx context.Context, cfg PublisherConfig) (Publisher, *nats.Conn) {
	if cfg.ConnectWait == 0 {
		cfg.ConnectWait = 5 * time.Second
	}

	if cfg.NATSURL == "" {
		slog.InfoContext(ctx, "audit: NATS_URL not configured, using noop publisher",
			slog.String("service", cfg.ServiceName))
		return NoopPublisher{}, nil
	}

	nc, err := nats.Connect(cfg.NATSURL,
		nats.Name(cfg.ServiceName+"-audit"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		slog.WarnContext(ctx, "audit: NATS connect failed, using noop publisher",
			slog.String("service", cfg.ServiceName),
			slog.String("url", cfg.NATSURL),
			slog.String("error", err.Error()))
		return NoopPublisher{}, nil
	}

	if err := waitForConn(ctx, nc, cfg.ConnectWait); err != nil {
		slog.WarnContext(ctx, "audit: NATS not ready within timeout, using noop publisher",
			slog.String("service", cfg.ServiceName),
			slog.String("url", cfg.NATSURL),
			slog.String("error", err.Error()))
		nc.Close()
		return NoopPublisher{}, nil
	}

	pub, err := NewNATSPublisher(nc)
	if err != nil {
		slog.WarnContext(ctx, "audit: JetStream setup failed, using noop publisher",
			slog.String("service", cfg.ServiceName),
			slog.String("error", err.Error()))
		_ = nc.Drain()
		return NoopPublisher{}, nil
	}

	slog.InfoContext(ctx, "audit: NATS connected",
		slog.String("service", cfg.ServiceName),
		slog.String("url", cfg.NATSURL))
	return Async(pub), nc
}

// waitForConn blocks until nc.IsConnected() or the timeout is reached.
// nats.RetryOnFailedConnect makes nats.Connect return before the TCP handshake
// completes, so JetStream operations must not be attempted until this returns nil.
func waitForConn(ctx context.Context, nc *nats.Conn, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for !nc.IsConnected() {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil
}
