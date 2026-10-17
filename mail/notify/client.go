package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// streamName is the JetStream stream that carries client notification events.
const streamName = "CLIENT_NOTIFY"

const (
	defaultConnectTimeout = 10 * time.Second
	defaultRetryBase      = time.Second
	defaultRetryMax       = 30 * time.Second
)

// ErrStop tells [Client.Consume] to stop consuming. When a handler returns an
// error that matches ErrStop, the event is acknowledged, consumption stops, and
// Consume returns nil. Use errors.Is to detect it.
var ErrStop = errors.New("notify: stop consuming")

// ErrClosed reports that the client was closed while an operation was running.
var ErrClosed = errors.New("notify: client is closed")

// Handler processes one notification event. Returning nil acknowledges the
// event; returning an error leaves it unacknowledged so the server redelivers
// it with backoff; returning an error matching [ErrStop] acknowledges the event
// and stops consumption.
type Handler func(ctx context.Context, event Event) error

// Client consumes one listener's client notification events. It is safe for
// concurrent use by multiple goroutines.
//
// Use [New] to construct a Client and [Client.Close] to release its connection.
type Client struct {
	source       func(ctx context.Context) (eventSource, error)
	consumerName string
	errorHandler func(error)
	done         chan struct{}

	closeOnce sync.Once
	closeInit func()
}

// Option configures a [Client] created by [New].
type Option func(*config) error

type config struct {
	url          string
	username     string
	secret       string
	listenerID   string
	consumer     string
	tlsConfig    *tls.Config
	timeout      time.Duration
	errorHandler func(error)
}

// New connects to the mxRaven client notification tier and returns a Client
// bound to one listener's durable consumer.
//
// A WebSocket URL, the listener's API key, and the listener identifier are
// required. Configure them with [WithURL], [WithCredentials], and [WithListener].
// The API key is the same credential used for SMTP submission and feedback.
//
// New returns an error if a required option is missing or invalid, or if the
// server rejects the API key. It does not consume events; call [Client.Consume]
// or [Client.Events].
func New(opts ...Option) (*Client, error) {
	cfg := config{
		timeout: defaultConnectTimeout,
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(cfg.url) == "" {
		return nil, errors.New("notify: URL is required (use WithURL)")
	}
	if !isWebSocketURL(cfg.url) {
		return nil, fmt.Errorf("notify: URL %q must use the ws or wss scheme", cfg.url)
	}
	if strings.TrimSpace(cfg.username) == "" || cfg.secret == "" {
		return nil, errors.New("notify: credentials are required (use WithCredentials)")
	}
	if strings.TrimSpace(cfg.listenerID) == "" {
		return nil, errors.New("notify: listener ID is required (use WithListener)")
	}
	consumer := cfg.consumer
	if consumer == "" {
		consumer = defaultConsumer(cfg.listenerID)
	}

	conn, err := nats.Connect(cfg.url, natsOptions(cfg)...)
	if err != nil {
		return nil, fmt.Errorf("notify: connect: %w", err)
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("notify: create jetstream context: %w", err)
	}

	client := &Client{
		consumerName: consumer,
		errorHandler: cfg.errorHandler,
		done:         make(chan struct{}),
	}
	client.source = func(ctx context.Context) (eventSource, error) {
		source, err := js.Consumer(ctx, streamName, consumer)
		if err != nil {
			return nil, fmt.Errorf("notify: resolve consumer %q: %w", consumer, err)
		}
		return source, nil
	}
	client.closeInit = conn.Close
	return client, nil
}

// WithURL sets the client notification tier WebSocket URL, for example
// "wss://nats.mxraven.com". Only the ws and wss schemes are accepted.
func WithURL(url string) Option {
	return func(cfg *config) error {
		if strings.TrimSpace(url) == "" {
			return errors.New("notify: URL must not be empty")
		}
		cfg.url = url
		return nil
	}
}

// WithCredentials sets the listener API key used to authenticate. The username
// is the key's prefix, for example "mxr_tx_ab12cd34ef56", and the secret is the
// key's secret.
//
// Both values must be non-empty.
func WithCredentials(username, secret string) Option {
	return func(cfg *config) error {
		if strings.TrimSpace(username) == "" {
			return errors.New("notify: username must not be empty")
		}
		if secret == "" {
			return errors.New("notify: secret must not be empty")
		}
		cfg.username = username
		cfg.secret = secret
		return nil
	}
}

// WithListener sets the listener whose events the client consumes. The value is
// used to derive the durable consumer name and the per-listener inbox prefix,
// so it must be the listener's identifier and must not contain a NATS subject
// wildcard or whitespace.
//
// The consumer name defaults to "notify-<listener>"; the control plane returns
// the same value from its notification-setting resource.
func WithListener(listenerID string) Option {
	return func(cfg *config) error {
		if strings.TrimSpace(listenerID) == "" {
			return errors.New("notify: listener ID must not be empty")
		}
		if !validToken(listenerID) {
			return fmt.Errorf("notify: listener ID %q must not contain '.', '*', '>', or whitespace", listenerID)
		}
		cfg.listenerID = listenerID
		return nil
	}
}

// WithConsumer overrides the durable consumer name. Use it to bind to the
// consumer name the control plane reports for the listener instead of relying
// on the derived default.
func WithConsumer(consumer string) Option {
	return func(cfg *config) error {
		if !validToken(consumer) {
			return fmt.Errorf("notify: consumer name %q must not contain '.', '*', '>', or whitespace", consumer)
		}
		cfg.consumer = consumer
		return nil
	}
}

// WithTLSConfig sets the TLS configuration used for the WebSocket handshake.
// When omitted, the default configuration verifies the server certificate
// against the system roots. Use it to trust a private certificate authority.
func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(cfg *config) error {
		if tlsConfig == nil {
			return errors.New("notify: TLS config must not be nil")
		}
		cfg.tlsConfig = tlsConfig
		return nil
	}
}

// WithConnectTimeout sets the timeout for establishing the WebSocket
// connection. The default is 10 seconds.
func WithConnectTimeout(timeout time.Duration) Option {
	return func(cfg *config) error {
		if timeout <= 0 {
			return errors.New("notify: connect timeout must be positive")
		}
		cfg.timeout = timeout
		return nil
	}
}

// WithErrorHandler sets a callback for non-fatal errors: connection-level NATS
// errors and events that cannot be decoded. It is invoked from the consuming
// goroutine and must return promptly.
//
// When no handler is set, an undecodable event stops [Client.Consume] or
// [Client.Events] with the decode error instead of being silently dropped.
func WithErrorHandler(handler func(error)) Option {
	return func(cfg *config) error {
		if handler == nil {
			return errors.New("notify: error handler must not be nil")
		}
		cfg.errorHandler = handler
		return nil
	}
}

// Close releases the client's connection. It is safe to call more than once.
//
// Active [Client.Consume] calls return [ErrClosed]; active [Client.Events]
// iterators stop. Events that were not acknowledged are redelivered later.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		close(c.done)
		if c.closeInit != nil {
			c.closeInit()
		}
	})
	return nil
}

// Consume reads events and dispatches each to handler until ctx is cancelled,
// the client is closed, the handler returns [ErrStop], or consumption fails.
//
// An event is acknowledged after handler returns nil, redelivered with
// exponential backoff after handler returns another error, and terminated when
// it cannot be decoded. Consume returns nil after a handler stop, ctx.Err()
// after cancellation, [ErrClosed] when the client is closed, and a wrapped
// error if the consumer stops on its own.
func (c *Client) Consume(ctx context.Context, handler Handler) error {
	if c == nil {
		return ErrClosed
	}
	if handler == nil {
		return errors.New("notify: handler is nil")
	}
	source, err := c.resolve(ctx)
	if err != nil {
		return err
	}

	stop := make(chan struct{})
	var stopOnce sync.Once
	var errMu sync.Mutex
	var consumeErr error
	consumeCtx, err := source.Consume(func(msg jetstream.Msg) {
		c.dispatch(ctx, msg, handler, func() { stopOnce.Do(func() { close(stop) }) })
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		errMu.Lock()
		consumeErr = err
		errMu.Unlock()
		c.report(err)
	}))
	if err != nil {
		return fmt.Errorf("notify: start consume: %w", err)
	}
	defer consumeCtx.Stop()

	select {
	case <-stop:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrClosed
	case <-consumeCtx.Closed():
		// The consumer stopped without the caller asking. Surface the terminal
		// error rather than reporting a clean end.
		select {
		case <-stop:
			return nil
		default:
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if clientClosed(c) {
			return ErrClosed
		}
		errMu.Lock()
		defer errMu.Unlock()
		if consumeErr != nil {
			return fmt.Errorf("notify: consume: %w", consumeErr)
		}
		return nil
	}
}

// dispatch decodes and handles one message.
func (c *Client) dispatch(ctx context.Context, msg jetstream.Msg, handler Handler, stop func()) {
	event, err := decodeEvent(msg.Subject(), msg.Data())
	if err != nil {
		// A body the SDK cannot decode is a protocol mismatch, not a transient
		// failure, so it is terminated rather than redelivered forever. It stops
		// consumption unless an error handler takes responsibility for it.
		_ = msg.Term()
		if c.errorHandler == nil {
			stop()
		}
		c.report(err)
		return
	}
	if err := handler(ctx, event); err != nil {
		if errors.Is(err, ErrStop) {
			_ = msg.Ack()
			stop()
			return
		}
		if err := msg.NakWithDelay(retryDelay(msg)); err != nil {
			c.report(fmt.Errorf("notify: retry event: %w", err))
		}
		return
	}
	if err := msg.Ack(); err != nil {
		c.report(fmt.Errorf("notify: acknowledge event: %w", err))
	}
}

// resolve returns the listener's durable consumer.
func (c *Client) resolve(ctx context.Context) (eventSource, error) {
	if c.source == nil {
		return nil, errors.New("notify: client is not initialized")
	}
	return c.source(ctx)
}

// report forwards a non-fatal error to the configured handler.
func (c *Client) report(err error) {
	if err == nil || c.errorHandler == nil {
		return
	}
	c.errorHandler(err)
}

// natsOptions builds the connection options. Credentials never appear in
// errors: nats.go reports an authorization failure without echoing them.
func natsOptions(cfg config) []nats.Option {
	options := []nats.Option{
		nats.UserInfo(cfg.username, cfg.secret),
		nats.CustomInboxPrefix("_INBOX." + cfg.listenerID),
		nats.Timeout(cfg.timeout),
		// A notification consumer is long-lived; keep reconnecting rather than
		// giving up after the library default of 60 attempts.
		nats.MaxReconnects(-1),
		nats.PermissionErrOnSubscribe(true),
		nats.Name("mxraven-notify"),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) { cfg.report(err) }),
	}
	if cfg.tlsConfig != nil {
		options = append(options, nats.Secure(cfg.tlsConfig))
	}
	return options
}

func (cfg config) report(err error) {
	if err == nil || cfg.errorHandler == nil {
		return
	}
	cfg.errorHandler(err)
}

// retryDelay returns the redelivery delay for a failed handler, growing with
// the number of deliveries and capped so a persistent failure settles into a
// slow retry.
func retryDelay(msg jetstream.Msg) time.Duration {
	metadata, err := msg.Metadata()
	if err != nil || metadata.NumDelivered <= 1 {
		return defaultRetryBase
	}
	delay := defaultRetryBase
	for i := uint64(1); i < metadata.NumDelivered && delay < defaultRetryMax; i++ {
		delay *= 2
	}
	if delay > defaultRetryMax {
		delay = defaultRetryMax
	}
	return delay
}

// defaultConsumer returns the durable consumer name for a listener.
func defaultConsumer(listenerID string) string {
	return "notify-" + listenerID
}

// isWebSocketURL reports whether url starts with a ws or wss scheme.
func isWebSocketURL(url string) bool {
	lower := strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://")
}

// validToken reports whether value is safe as a NATS subject token and a
// JetStream consumer name.
func validToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r == '.' || r == '*' || r == '>' || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
