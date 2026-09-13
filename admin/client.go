package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/synqronlabs/mxraven-go/admin/api"
)

// Client is a mxRaven control-plane v2 API client.
//
// It embeds the generated [api.Client], so every operation described by the
// authoritative contract is available directly. Client and its embedded client
// are safe for concurrent use by multiple goroutines.
type Client struct {
	*api.Client
}

// Option configures a [Client] created by [New].
type Option func(*config) error

type config struct {
	baseURL     string
	httpClient  *http.Client
	tokenSource func(ctx context.Context, operation string) (string, error)
	userAgent   string
	retry       RetryPolicy
}

// New creates a control-plane client.
//
// A base URL and an authentication source are required. The base URL should
// include the scheme and host, for example "https://control.example.com".
// Configure authentication with [WithToken] or [WithTokenSource].
//
// New returns an error if any option is invalid or if a required option is
// missing.
func New(opts ...Option) (*Client, error) {
	cfg := config{
		httpClient: http.DefaultClient,
		retry:      DefaultRetryPolicy,
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if cfg.baseURL == "" {
		return nil, errors.New("admin: base URL is required (use WithBaseURL)")
	}
	if cfg.tokenSource == nil {
		return nil, errors.New("admin: authentication is required (use WithToken or WithTokenSource)")
	}

	httpClient := *cfg.httpClient
	httpClient.Transport = buildTransport(httpClient.Transport, cfg)

	client, err := api.NewClient(cfg.baseURL, securitySource{token: cfg.tokenSource}, api.WithClient(&httpClient))
	if err != nil {
		return nil, fmt.Errorf("admin: create client: %w", err)
	}
	return &Client{Client: client}, nil
}

// WithBaseURL sets the API base URL, for example
// "https://control.example.com". The value must not be empty.
func WithBaseURL(baseURL string) Option {
	return func(cfg *config) error {
		if strings.TrimSpace(baseURL) == "" {
			return errors.New("admin: base URL must not be empty")
		}
		cfg.baseURL = baseURL
		return nil
	}
}

// WithToken authenticates every request with a static bearer token. The token
// must not be empty.
//
// Prefer [WithTokenSource] when tokens expire and must be refreshed.
func WithToken(token string) Option {
	return func(cfg *config) error {
		if token == "" {
			return errors.New("admin: token must not be empty")
		}
		cfg.tokenSource = func(context.Context, string) (string, error) {
			return token, nil
		}
		return nil
	}
}

// WithTokenSource authenticates every request with a token obtained from fn.
//
// The operation argument is the generated operation name, which allows a
// source to select credentials per operation. fn is invoked for every request
// and must return a non-empty token; a returned error aborts the request
// before it is sent. fn must not be nil.
func WithTokenSource(fn func(ctx context.Context, operation string) (string, error)) Option {
	return func(cfg *config) error {
		if fn == nil {
			return errors.New("admin: token source must not be nil")
		}
		cfg.tokenSource = fn
		return nil
	}
}

// WithHTTPClient sets the HTTP client used for requests. This is useful for
// configuring timeouts, proxies, and custom transports. An explicit timeout is
// not applied by default. The client must not be nil.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(cfg *config) error {
		if httpClient == nil {
			return errors.New("admin: HTTP client must not be nil")
		}
		cfg.httpClient = httpClient
		return nil
	}
}

// WithUserAgent sets the User-Agent header sent with every request. It is
// ignored when the request already carries a User-Agent header.
func WithUserAgent(userAgent string) Option {
	return func(cfg *config) error {
		cfg.userAgent = userAgent
		return nil
	}
}

// WithRetry overrides the automatic retry policy. Retries apply to
// rate-limited responses and honour the server's Retry-After header. Pass a
// policy with MaxAttempts less than or equal to 1 to disable retries.
//
// The default policy is [DefaultRetryPolicy].
func WithRetry(policy RetryPolicy) Option {
	return func(cfg *config) error {
		cfg.retry = policy
		return nil
	}
}

// buildTransport layers the User-Agent and retry transports over base.
func buildTransport(base http.RoundTripper, cfg config) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if cfg.userAgent != "" {
		base = &userAgentTransport{base: base, userAgent: cfg.userAgent}
	}
	if policy := cfg.retry.normalized(); policy.MaxAttempts > 1 {
		base = newRetryTransport(base, policy)
	}
	return base
}

// securitySource adapts the configured token source to ogen's SecuritySource.
type securitySource struct {
	token func(ctx context.Context, operation string) (string, error)
}

// BearerAuth implements [api.SecuritySource].
func (s securitySource) BearerAuth(ctx context.Context, operation api.OperationName) (api.BearerAuth, error) {
	token, err := s.token(ctx, operation)
	if err != nil {
		return api.BearerAuth{}, fmt.Errorf("admin: obtain bearer token: %w", err)
	}
	if token == "" {
		return api.BearerAuth{}, errors.New("admin: token source returned an empty token")
	}
	return api.BearerAuth{Token: token}, nil
}

// userAgentTransport sets a default User-Agent header on outgoing requests.
type userAgentTransport struct {
	base      http.RoundTripper
	userAgent string
}

// RoundTrip implements [http.RoundTripper].
func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.userAgent)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
