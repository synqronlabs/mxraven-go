package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	ravenclient "github.com/synqronlabs/raven/client"
	ravenmail "github.com/synqronlabs/raven/mail"
)

// DefaultAddressPort is the mxRaven submission port.
const DefaultAddressPort = 587

const defaultPoolSize = 5

// Client submits mail to the mxRaven SMTP submission service.
//
// A Client maintains a bounded pool of authenticated SMTP connections and is
// safe for concurrent use. Use [New] to construct one and [Client.Close] to
// release its connections.
type Client struct {
	pool *ravenclient.Pool
}

// Option configures a [Client] created by [New].
type Option func(*config) error

type config struct {
	host           string
	port           int
	username       string
	secret         string
	tlsConfig      *tls.Config
	poolSize       int
	connectTimeout time.Duration
	localName      string
}

// New creates a submission client.
//
// An address and credentials are required. Configure them with
// [WithAddress] and [WithCredentials].
func New(opts ...Option) (*Client, error) {
	cfg := config{
		port:           DefaultAddressPort,
		poolSize:       defaultPoolSize,
		connectTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(cfg.host) == "" {
		return nil, errors.New("mail: server address is required (use WithAddress)")
	}
	if cfg.port <= 0 || cfg.port > 65535 {
		return nil, fmt.Errorf("mail: invalid server port %d", cfg.port)
	}
	if strings.TrimSpace(cfg.username) == "" || cfg.secret == "" {
		return nil, errors.New("mail: credentials are required (use WithCredentials)")
	}
	if cfg.poolSize <= 0 {
		return nil, fmt.Errorf("mail: invalid pool size %d", cfg.poolSize)
	}

	dialer := ravenclient.NewDialer(cfg.host, cfg.port)
	dialer.TLSConfig = cfg.tlsConfig
	dialer.StartTLS = true
	dialer.RequireTLS = true
	dialer.ConnectTimeout = cfg.connectTimeout
	dialer.LocalName = cfg.localName
	dialer.Auth = &ravenclient.ClientAuth{
		Username: cfg.username,
		Password: cfg.secret,
	}

	return &Client{pool: ravenclient.NewPool(dialer, cfg.poolSize)}, nil
}

// WithAddress sets the submission server host and port. The host must not
// include a port.
//
// The default port is [DefaultAddressPort].
func WithAddress(host string, port int) Option {
	return func(cfg *config) error {
		cfg.host = host
		cfg.port = port
		return nil
	}
}

// WithCredentials sets the submission API key used to authenticate. The
// username is the key's username, for example "mxr_tx_ab12cd34ef56", and the
// secret is the key's secret.
//
// Both values must be non-empty.
func WithCredentials(username, secret string) Option {
	return func(cfg *config) error {
		if strings.TrimSpace(username) == "" {
			return errors.New("mail: username must not be empty")
		}
		if secret == "" {
			return errors.New("mail: secret must not be empty")
		}
		cfg.username = username
		cfg.secret = secret
		return nil
	}
}

// WithTLSConfig sets the TLS configuration used for the STARTTLS handshake.
// When omitted, the default configuration requires TLS 1.2 or later and
// verifies the server certificate against the system roots using the
// configured host as the server name.
func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(cfg *config) error {
		if tlsConfig == nil {
			return errors.New("mail: TLS config must not be nil")
		}
		cfg.tlsConfig = tlsConfig
		return nil
	}
}

// WithPoolSize sets the maximum number of SMTP connections the client keeps
// open. The value must be positive. The default is 5.
func WithPoolSize(size int) Option {
	return func(cfg *config) error {
		cfg.poolSize = size
		return nil
	}
}

// WithConnectTimeout sets the timeout for establishing a TCP connection. The
// default is 30 seconds.
func WithConnectTimeout(timeout time.Duration) Option {
	return func(cfg *config) error {
		if timeout <= 0 {
			return errors.New("mail: connect timeout must be positive")
		}
		cfg.connectTimeout = timeout
		return nil
	}
}

// WithLocalName sets the name sent in the EHLO command. When empty the raven
// default, "localhost", is used.
func WithLocalName(name string) Option {
	return func(cfg *config) error {
		cfg.localName = name
		return nil
	}
}

// Send submits a composed message and returns the server's result.
//
// A non-nil [*Result] is returned alongside the error when the server replied
// but rejected the transaction, because per-recipient detail remains useful.
// On success the error is nil.
func (c *Client) Send(ctx context.Context, msg *Message) (*Result, error) {
	if c == nil || c.pool == nil {
		return nil, errors.New("mail: client is not initialized")
	}
	if msg == nil {
		return nil, errors.New("mail: message is nil")
	}
	built, err := msg.build()
	if err != nil {
		return nil, err
	}
	return c.transact(ctx, func(client *ravenclient.Client) (*ravenclient.SendResult, error) {
		return client.Send(built)
	})
}

// Envelope is the SMTP envelope for a raw message, independent of the message
// headers. Use it with [Client.SendRaw] to stream an already serialized
// RFC 5322 message.
type Envelope struct {
	// From is the envelope sender. An empty value requests a null reverse-path,
	// which is appropriate for bounce messages.
	From string
	// To holds at least one envelope recipient.
	To []string
}

// SendRaw streams a raw RFC 5322 message with an explicit envelope.
//
// The message is not parsed, so the caller is responsible for RFC 5322
// correctness. This is the efficient path for large or pre-rendered messages.
func (c *Client) SendRaw(ctx context.Context, envelope Envelope, data io.Reader) (*Result, error) {
	if c == nil || c.pool == nil {
		return nil, errors.New("mail: client is not initialized")
	}
	if data == nil {
		return nil, errors.New("mail: message data is nil")
	}
	ravenEnvelope, err := envelope.raven()
	if err != nil {
		return nil, err
	}
	return c.transact(ctx, func(client *ravenclient.Client) (*ravenclient.SendResult, error) {
		return client.SendRaw(ravenEnvelope, data)
	})
}

// Close releases the client's pooled connections. It is safe to call more than
// once.
func (c *Client) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// transact runs send against a pooled connection. A failed command can leave
// unread replies on the wire, so the connection is closed instead of being
// reused; returning it to the pool still releases its slot for a later dial.
func (c *Client) transact(ctx context.Context, send func(*ravenclient.Client) (*ravenclient.SendResult, error)) (*Result, error) {
	client, err := c.pool.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail: acquire connection: %w", err)
	}
	result, sendErr := send(client)
	if sendErr != nil {
		_ = client.Close()
		c.pool.Put(client)
		return resultFrom(result), translateSMTPError(sendErr)
	}
	c.pool.Put(client)
	return resultFrom(result), nil
}

// raven converts the envelope into a raven envelope.
func (e Envelope) raven() (ravenmail.Envelope, error) {
	var envelope ravenmail.Envelope
	if from := strings.TrimSpace(e.From); from != "" {
		address, err := ravenmail.ParseAddress(from)
		if err != nil {
			return envelope, fmt.Errorf("mail: parse envelope sender %q: %w", from, err)
		}
		envelope.From = ravenmail.Path{Mailbox: address}
	}
	if len(e.To) == 0 {
		return envelope, errors.New("mail: envelope requires at least one recipient")
	}
	envelope.To = make([]ravenmail.Recipient, 0, len(e.To))
	for _, recipient := range e.To {
		address, err := ravenmail.ParseAddress(recipient)
		if err != nil {
			return envelope, fmt.Errorf("mail: parse envelope recipient %q: %w", recipient, err)
		}
		envelope.To = append(envelope.To, ravenmail.Recipient{
			Address: ravenmail.Path{Mailbox: address},
		})
	}
	return envelope, nil
}
