package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultHTTPTimeout bounds non-streaming feedback requests.
const defaultHTTPTimeout = 30 * time.Second

// Client calls the mxRaven feedback service.
//
// A Client is safe for concurrent use.
type Client struct {
	baseURL    string
	username   string
	secret     string
	httpClient *http.Client
}

// Option configures a [Client] created by [New].
type Option func(*config) error

type config struct {
	baseURL    string
	username   string
	secret     string
	httpClient *http.Client
}

// New creates a feedback client. A base URL is required. Credentials are
// required by the learning methods and are configured with [WithCredentials].
func New(opts ...Option) (*Client, error) {
	cfg := config{
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(cfg.baseURL) == "" {
		return nil, errors.New("feedback: base URL is required (use WithBaseURL)")
	}
	return &Client{
		baseURL:    strings.TrimRight(cfg.baseURL, "/"),
		username:   cfg.username,
		secret:     cfg.secret,
		httpClient: cfg.httpClient,
	}, nil
}

// WithBaseURL sets the feedback service base URL, for example
// "https://feedback.mxraven.com". The value must not be empty.
func WithBaseURL(baseURL string) Option {
	return func(cfg *config) error {
		if strings.TrimSpace(baseURL) == "" {
			return errors.New("feedback: base URL must not be empty")
		}
		cfg.baseURL = baseURL
		return nil
	}
}

// WithCredentials sets the API key used to authenticate learning requests.
// The username is the key's username and the secret is the key's secret.
//
// Both values must be non-empty.
func WithCredentials(username, secret string) Option {
	return func(cfg *config) error {
		if strings.TrimSpace(username) == "" {
			return errors.New("feedback: username must not be empty")
		}
		if secret == "" {
			return errors.New("feedback: secret must not be empty")
		}
		cfg.username = username
		cfg.secret = secret
		return nil
	}
}

// WithHTTPClient sets the HTTP client used for requests. The default has a
// 30-second timeout. The client must not be nil.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(cfg *config) error {
		if httpClient == nil {
			return errors.New("feedback: HTTP client must not be nil")
		}
		cfg.httpClient = httpClient
		return nil
	}
}

// Disposition selects the training label for a message.
type Disposition string

const (
	// DispositionSpam marks the message as spam.
	DispositionSpam Disposition = "spam"
	// DispositionHam marks the message as not spam.
	DispositionHam Disposition = "ham"
)

// LearningResult reports the outcome of a successful learning request.
type LearningResult struct {
	// Status is the service status, normally "learned".
	Status string `json:"status"`
	// Disposition is the training label that was applied.
	Disposition Disposition `json:"disposition"`
	// TenantID is the tenant that owns the matched message.
	TenantID string `json:"tenant_id"`
	// ListenerID is the listener that processed the matched message.
	ListenerID string `json:"listener_id"`
	// MatchedHashKind identifies which stored hash matched the submitted bytes.
	MatchedHashKind string `json:"matched_hash_kind"`
}

// LearnSpam teaches the spam filter that rawMIME is spam.
func (c *Client) LearnSpam(ctx context.Context, rawMIME io.Reader) (*LearningResult, error) {
	return c.Learn(ctx, DispositionSpam, rawMIME)
}

// LearnHam teaches the spam filter that rawMIME is not spam.
func (c *Client) LearnHam(ctx context.Context, rawMIME io.Reader) (*LearningResult, error) {
	return c.Learn(ctx, DispositionHam, rawMIME)
}

// Learn submits one training example with the given disposition.
//
// rawMIME must be the exact raw RFC 822 bytes that mxRaven processed; the
// service matches them against stored evidence by SHA-256. A message with no
// matching evidence returns an [*Error] with Status 404.
func (c *Client) Learn(ctx context.Context, disposition Disposition, rawMIME io.Reader) (*LearningResult, error) {
	if disposition != DispositionSpam && disposition != DispositionHam {
		return nil, fmt.Errorf("feedback: invalid disposition %q", disposition)
	}
	if c.username == "" || c.secret == "" {
		return nil, errors.New("feedback: credentials are required for learning (use WithCredentials)")
	}
	if rawMIME == nil {
		return nil, errors.New("feedback: raw MIME is nil")
	}

	endpoint := c.baseURL + "/v1/feedback/learn/" + string(disposition)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, rawMIME)
	if err != nil {
		return nil, fmt.Errorf("feedback: build learning request: %w", err)
	}
	req.Header.Set("Content-Type", "message/rfc822")
	req.SetBasicAuth(c.username, c.secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feedback: submit learning request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, newHTTPError(resp)
	}

	var result LearningResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxErrorBody)).Decode(&result); err != nil {
		return nil, fmt.Errorf("feedback: decode learning response: %w", err)
	}
	return &result, nil
}

// Unsubscribe performs an RFC 8058 one-click unsubscribe for a token. It is the
// operation a recipient mail client performs against the List-Unsubscribe URL;
// applications rarely call it directly.
func (c *Client) Unsubscribe(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("feedback: unsubscribe token is empty")
	}

	endpoint := c.baseURL + "/v1/feedback/unsubscribe/" + url.PathEscape(token)
	body := strings.NewReader("List-Unsubscribe=One-Click")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("feedback: build unsubscribe request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("feedback: submit unsubscribe request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return newHTTPError(resp)
	}
	return nil
}
