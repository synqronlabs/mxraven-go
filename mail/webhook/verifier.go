package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Webhook signature headers. HTTP header names are case-insensitive; these
// constants carry the canonical spelling used by mxRaven.
const (
	// HeaderWebhookID carries the delivery ID, which is the delivery task ID.
	HeaderWebhookID = "X-MxRaven-Webhook-ID"
	// HeaderTimestamp carries the Unix signing time in seconds.
	HeaderTimestamp = "X-MxRaven-Timestamp"
	// HeaderSignature carries the "sha256=<hex>" HMAC.
	HeaderSignature = "X-MxRaven-Signature"
	// HeaderSignatureKID carries the signing key ID.
	HeaderSignatureKID = "X-MxRaven-Signature-Kid"
)

// ErrInvalidSignature reports that a request's signature did not match.
var ErrInvalidSignature = errors.New("webhook: invalid signature")

const (
	defaultTolerance   = 5 * time.Minute
	defaultMaxBodySize = 1 << 20
)

// Verifier verifies the signature of an mxRaven webhook request.
//
// A Verifier is safe for concurrent use once constructed. Configure it with a
// signing secret using [WithSecret] or with per-key secrets using [WithKey].
type Verifier struct {
	secret    string
	keys      map[string]string
	tolerance time.Duration
	maxBody   int64
	now       func() time.Time
}

// VerifierOption configures a [Verifier].
type VerifierOption func(*Verifier) error

// WithSecret sets the signing secret used to verify requests. The secret is
// used as literal key bytes; do not decode it. The value is used when no
// key-specific secret matches.
func WithSecret(secret string) VerifierOption {
	return func(v *Verifier) error {
		if secret == "" {
			return errors.New("webhook: signing secret must not be empty")
		}
		v.secret = secret
		return nil
	}
}

// WithKey registers a signing secret for a specific signature key ID. Use it
// to accept deliveries from more than one key, for example during rotation.
// The secret is used as literal key bytes.
func WithKey(kid, secret string) VerifierOption {
	return func(v *Verifier) error {
		if strings.TrimSpace(kid) == "" {
			return errors.New("webhook: signing key ID must not be empty")
		}
		if secret == "" {
			return errors.New("webhook: signing secret must not be empty")
		}
		if v.keys == nil {
			v.keys = make(map[string]string)
		}
		v.keys[kid] = secret
		return nil
	}
}

// WithTolerance sets the maximum accepted difference between the request
// timestamp and the current time. A zero value disables timestamp checking.
// The default is 5 minutes.
func WithTolerance(tolerance time.Duration) VerifierOption {
	return func(v *Verifier) error {
		if tolerance < 0 {
			return errors.New("webhook: tolerance must not be negative")
		}
		v.tolerance = tolerance
		return nil
	}
}

// WithMaxBodyBytes sets the maximum request body size that Verify reads.
// Bodies larger than the limit are rejected. The default is 1 MiB.
func WithMaxBodyBytes(max int64) VerifierOption {
	return func(v *Verifier) error {
		if max <= 0 {
			return errors.New("webhook: maximum body size must be positive")
		}
		v.maxBody = max
		return nil
	}
}

// NewVerifier creates a Verifier. At least one secret is required.
func NewVerifier(opts ...VerifierOption) (*Verifier, error) {
	verifier := &Verifier{
		tolerance: defaultTolerance,
		maxBody:   defaultMaxBodySize,
		now:       time.Now,
		keys:      make(map[string]string),
	}
	for _, opt := range opts {
		if err := opt(verifier); err != nil {
			return nil, err
		}
	}
	if verifier.secret == "" && len(verifier.keys) == 0 {
		return nil, errors.New("webhook: a signing secret is required (use WithSecret or WithKey)")
	}
	return verifier, nil
}

// Verify verifies the signature of r.
//
// The request body is read and restored, so the caller can decode it after
// verification. It returns [ErrInvalidSignature] when the signature does not
// match, and a descriptive error when required headers are missing or the
// request is otherwise unusable.
func (v *Verifier) Verify(r *http.Request) error {
	body, err := v.readBody(r)
	if err != nil {
		return err
	}
	return v.verify(r, body)
}

// VerifyAndDecode verifies r and decodes its payload in one step.
func (v *Verifier) VerifyAndDecode(r *http.Request) (Event, error) {
	body, err := v.readBody(r)
	if err != nil {
		return Event{}, err
	}
	if err := v.verify(r, body); err != nil {
		return Event{}, err
	}
	return Decode(body)
}

// readBody reads and restores the request body, enforcing the configured size
// limit.
func (v *Verifier) readBody(r *http.Request) ([]byte, error) {
	if r == nil {
		return nil, errors.New("webhook: request is nil")
	}
	if r.Body == nil {
		return nil, errors.New("webhook: request has no body")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, v.maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("webhook: read request body: %w", err)
	}
	if int64(len(body)) > v.maxBody {
		return nil, fmt.Errorf("webhook: request body exceeds %d bytes", v.maxBody)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// verify checks the signature headers against body. The body has already been
// read.
func (v *Verifier) verify(r *http.Request, body []byte) error {
	if r.URL == nil {
		return errors.New("webhook: request URL is missing")
	}
	webhookID := strings.TrimSpace(r.Header.Get(HeaderWebhookID))
	if webhookID == "" {
		return errors.New("webhook: missing webhook ID header")
	}
	timestamp := strings.TrimSpace(r.Header.Get(HeaderTimestamp))
	if timestamp == "" {
		return errors.New("webhook: missing timestamp header")
	}
	signature := strings.TrimSpace(r.Header.Get(HeaderSignature))
	if signature == "" {
		return errors.New("webhook: missing signature header")
	}

	if v.tolerance > 0 {
		seconds, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return fmt.Errorf("webhook: invalid timestamp %q", timestamp)
		}
		skew := v.now().Sub(time.Unix(seconds, 0))
		if skew < 0 {
			skew = -skew
		}
		if skew > v.tolerance {
			return errors.New("webhook: timestamp is outside the accepted clock skew")
		}
	}

	secret, err := v.secretFor(r.Header.Get(HeaderSignatureKID))
	if err != nil {
		return err
	}

	const scheme = "sha256="
	if !strings.HasPrefix(signature, scheme) {
		return errors.New("webhook: unsupported signature algorithm")
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, scheme))
	if err != nil {
		return errors.New("webhook: malformed signature")
	}
	if !hmac.Equal(provided, signatureSum(secret, r.Method, r.URL, timestamp, webhookID, body)) {
		return ErrInvalidSignature
	}
	return nil
}

// secretFor selects the secret for the presented key ID. When key-specific
// secrets are configured, the key ID must match one of them.
func (v *Verifier) secretFor(kid string) (string, error) {
	kid = strings.TrimSpace(kid)
	if len(v.keys) > 0 {
		if kid == "" {
			return "", errors.New("webhook: missing signature key ID")
		}
		secret, ok := v.keys[kid]
		if !ok {
			return "", fmt.Errorf("webhook: unknown signature key ID %q", kid)
		}
		return secret, nil
	}
	return v.secret, nil
}

// signatureSum computes the expected HMAC over the canonical request string.
//
// The canonical string is six newline-separated fields:
//
//	timestamp
//	webhookID
//	METHOD
//	lowercase host, including the port when present
//	escaped path plus raw query, or "/"
//	lowercase hex SHA-256 of the raw body
func signatureSum(secret, method string, target *url.URL, timestamp, webhookID string, body []byte) []byte {
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		timestamp,
		webhookID,
		method,
		strings.ToLower(target.Host),
		canonicalRequestTarget(target),
		hex.EncodeToString(bodyHash[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return mac.Sum(nil)
}

// canonicalRequestTarget returns the request target used in the signature.
func canonicalRequestTarget(target *url.URL) string {
	value := target.EscapedPath()
	if value == "" {
		value = "/"
	}
	if target.RawQuery != "" {
		value += "?" + target.RawQuery
	}
	return value
}
