package admin

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy configures automatic retries of rate-limited responses.
//
// Retries are limited to responses the server rejected before processing, so
// they are safe even for non-idempotent operations:
//
//   - 429 Too Many Requests is always retried.
//   - 503 Service Unavailable is retried only when it carries Retry-After.
//
// Transport errors are never retried, because their outcome is uncertain.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, including the first. A
	// value less than or equal to 1 disables retries.
	MaxAttempts int
	// BaseDelay is the initial backoff applied when the server does not send a
	// Retry-After value. It doubles with each attempt and is capped by
	// MaxDelay. Zero uses DefaultRetryPolicy.BaseDelay.
	BaseDelay time.Duration
	// MaxDelay caps any single wait, including a server-provided Retry-After.
	// Zero uses DefaultRetryPolicy.MaxDelay.
	MaxDelay time.Duration
}

// DefaultRetryPolicy is applied by [New] unless [WithRetry] overrides it. It
// retries rate-limited responses up to four total attempts.
var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts: 4,
	BaseDelay:   500 * time.Millisecond,
	MaxDelay:    30 * time.Second,
}

func (p RetryPolicy) normalized() RetryPolicy {
	if p.BaseDelay <= 0 {
		p.BaseDelay = DefaultRetryPolicy.BaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = DefaultRetryPolicy.MaxDelay
	}
	return p
}

// newRetryTransport wraps base with the given retry policy.
func newRetryTransport(base http.RoundTripper, policy RetryPolicy) *retryTransport {
	return &retryTransport{
		base:   base,
		policy: policy,
		now:    time.Now,
		sleep:  sleepContext,
	}
}

type retryTransport struct {
	base   http.RoundTripper
	policy RetryPolicy
	now    func() time.Time
	sleep  func(ctx context.Context, d time.Duration) error
}

// RoundTrip implements [http.RoundTripper].
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	policy := t.policy.normalized()
	attempts := max(policy.MaxAttempts, 1)

	current := req
	for attempt := 1; ; attempt++ {
		resp, err := t.base.RoundTrip(current)
		if err != nil {
			return nil, err
		}
		if attempt >= attempts || !shouldRetry(resp) || !canReplay(req) {
			return resp, nil
		}

		next, err := replayRequest(req)
		if err != nil {
			return resp, nil
		}

		delay := retryDelay(resp, attempt, policy, t.now())
		drainAndClose(resp.Body)
		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, err
		}
		current = next
	}
}

// shouldRetry reports whether a response represents a rejected request that is
// safe to send again.
func shouldRetry(resp *http.Response) bool {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusServiceUnavailable:
		return resp.Header.Get("Retry-After") != ""
	default:
		return false
	}
}

// canReplay reports whether the request body can be reconstructed for another
// attempt.
func canReplay(req *http.Request) bool {
	if req.Body == nil || req.Body == http.NoBody {
		return true
	}
	return req.GetBody != nil
}

// replayRequest returns a copy of req with a fresh body.
func replayRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return clone, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
}

// retryDelay returns the wait before the next attempt, honouring Retry-After
// when present and falling back to capped exponential backoff.
func retryDelay(resp *http.Response, attempt int, policy RetryPolicy, now time.Time) time.Duration {
	if delay, ok := parseRetryAfter(resp.Header.Get("Retry-After"), now); ok {
		return min(delay, policy.MaxDelay)
	}

	delay := policy.BaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= policy.MaxDelay {
			return policy.MaxDelay
		}
	}
	return min(delay, policy.MaxDelay)
}

// parseRetryAfter parses a Retry-After header expressed as delta seconds or as
// an HTTP date.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return max(time.Duration(seconds), 0) * time.Second, true
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(at.Sub(now), 0), true
	}
	return 0, false
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
