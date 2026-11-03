package client

import (
	"errors"
	"net/http"
	"time"
)

type clientOptions struct {
	timeout   time.Duration
	transport http.RoundTripper
	agent     string
}

// Option configures a newly constructed client; existing clients stay immutable.
type Option func(*clientOptions) error

func WithTimeout(timeout time.Duration) Option {
	return func(c *clientOptions) error {
		if timeout <= 0 || timeout > 10*time.Minute {
			return errors.New("HTTP timeout must be positive and at most 10m")
		}
		c.timeout = timeout
		return nil
	}
}
func WithTransport(transport http.RoundTripper) Option {
	return func(c *clientOptions) error {
		if transport == nil {
			return errors.New("HTTP transport is required")
		}
		c.transport = transport
		return nil
	}
}
func WithUserAgent(agent string) Option {
	return func(c *clientOptions) error {
		if len(agent) == 0 || len(agent) > 128 {
			return errors.New("user agent must contain 1..128 printable ASCII bytes")
		}
		for _, r := range agent {
			if r < 32 || r > 126 {
				return errors.New("user agent must contain printable ASCII")
			}
		}
		c.agent = agent
		return nil
	}
}

// CloseIdleConnections releases idle transport connections, never active requests.
func (c *Client) CloseIdleConnections() { c.http.CloseIdleConnections() }
