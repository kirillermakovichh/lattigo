package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"example.com/go-block-lattice/internal/event"
	"strconv"
	"time"
)

// eventRetryDelay uses a bounded, reproducible spread to avoid synchronized
// reconnects. It is transport scheduling, never a consensus input.
func eventRetryDelay(attempt uint, position string) time.Duration {
	if attempt > 5 {
		attempt = 5
	}
	base := (250 * time.Millisecond) << attempt
	sum := sha256.Sum256([]byte(position + ":" + strconv.FormatUint(uint64(attempt), 10)))
	return base*3/4 + time.Duration(sum[0])*base/1024
}
func retryEventError(err error) bool {
	var consumer *eventConsumerError
	if errors.As(err, &consumer) || errors.Is(err, ErrEventProtocol) || errors.Is(err, context.Canceled) {
		return false
	}
	var remote *Error
	if errors.As(err, &remote) {
		return remote.Status >= 500 || remote.Status == 429 || (remote.Status == 409 && remote.Code == "event_ahead")
	}
	return true
}

// Watch reconnects until cancellation or a permanent protocol/consumer error.
// No background goroutine is retained after this call returns.
func (c *Client) Watch(ctx context.Context, options EventOptions, consume func(Event) error) (string, error) {
	if consume == nil {
		return options.After, errors.New("event callback required")
	}
	if _, err := options.Network.Bytes(); err != nil {
		return options.After, err
	}
	if options.After != "" {
		if _, err := event.ParseID(options.After, options.Network); err != nil {
			return options.After, err
		}
	}
	attempt := uint(0)
	for {
		before := options.After
		last, err := c.Events(ctx, options, consume)
		options.After = last
		if ctx.Err() != nil {
			return last, ctx.Err()
		}
		if !retryEventError(err) {
			return last, err
		}
		if last != before {
			attempt = 0
		}
		timer := time.NewTimer(eventRetryDelay(attempt, last))
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, ctx.Err()
		case <-timer.C:
		}
		if attempt < 5 {
			attempt++
		}
	}
}
