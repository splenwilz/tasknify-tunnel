package client

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/splenwilz/devtunnel/pkg/config"
)

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
	jitterFraction = 0.25
)

// ConnectWithRetry connects to the tunnel server and automatically reconnects
// on disconnection with exponential backoff and jitter.
func ConnectWithRetry(ctx context.Context, cfg *config.ClientConfig, logger *slog.Logger, onConnect func(url string)) error {
	backoff := initialBackoff

	for {
		client := NewTunnelClient(cfg, logger)

		url, err := client.Connect(ctx)
		if err == nil {
			backoff = initialBackoff // Reset on success
			onConnect(url)
			err = client.Run() // Blocks until disconnected
			client.Close()
		} else {
			client.Close()
		}

		// Check if we should stop
		if ctx.Err() != nil {
			return ctx.Err()
		}

		logger.Warn("connection lost, reconnecting",
			"backoff", backoff.Round(time.Millisecond),
			"error", err,
		)

		// Wait with jitter
		jitter := time.Duration(float64(backoff) * jitterFraction * rand.Float64())
		select {
		case <-time.After(backoff + jitter):
		case <-ctx.Done():
			return ctx.Err()
		}

		// Exponential backoff
		backoff = min(backoff*2, maxBackoff)
	}
}
