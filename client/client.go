// Package client exposes the versioned Go Block Lattice HTTP client. It verifies
// block signatures but relies on a trusted node for confirmation certificates.
package client

import (
	"crypto/ed25519"
	"net/http"
	"time"

	transport "example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

type Client = transport.Client
type Option = transport.Option
type Error = transport.Error
type SubmissionError = transport.SubmissionError
type Status = transport.Status
type PeerDiagnostic = transport.PeerDiagnostic
type AccountState = transport.AccountState
type Receivable = transport.Receivable
type Submission = transport.Submission
type Event = transport.Event
type EventOptions = transport.EventOptions
type Confirmation = transport.Confirmation
type Anchor = transport.Anchor
type Page[T any] = transport.Page[T]
type ReadQuery = transport.ReadQuery
type ReadResult = transport.ReadResult
type ReadOperation = transport.ReadOperation
type Account = protocol.Account
type Hash = protocol.Hash
type Amount = protocol.Amount
type Block = protocol.Block
type Genesis = protocol.Genesis
type Allocation = protocol.Allocation

const (
	MaxResponseBytes = transport.MaxResponseBytes
	MaxBatchQueries  = transport.MaxBatchQueries
	ReadStatus       = transport.ReadStatus
	ReadAccount      = transport.ReadAccount
	ReadBlock        = transport.ReadBlock
)

func New(endpoint, token string) (*Client, error) { return transport.New(endpoint, token) }
func NewWithOptions(endpoint, token string, options ...Option) (*Client, error) {
	return transport.NewWithOptions(endpoint, token, options...)
}
func WithTimeout(timeout time.Duration) Option { return transport.WithTimeout(timeout) }
func WithTransport(roundTripper http.RoundTripper) Option {
	return transport.WithTransport(roundTripper)
}
func WithUserAgent(agent string) Option      { return transport.WithUserAgent(agent) }
func IsNotFound(err error) bool              { return transport.IsNotFound(err) }
func ParseAmount(raw string) (Amount, error) { return protocol.ParseAmount(raw) }
func Address(key ed25519.PublicKey) Account  { return protocol.Address(key) }
func Digest(raw []byte) Hash                 { return protocol.Digest(raw) }

var ErrEventProtocol = transport.ErrEventProtocol
