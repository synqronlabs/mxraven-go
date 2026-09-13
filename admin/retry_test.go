package admin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func httpResponse(status int, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

func TestRetryTransport(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	retryAfterHeader := func(value string) http.Header {
		return http.Header{"Retry-After": []string{value}}
	}

	tests := []struct {
		name        string
		responses   []*http.Response
		policy      RetryPolicy
		wantStatus  int
		wantDelays  []time.Duration
		wantNoRetry bool
	}{
		{
			name:       "retry after seconds",
			responses:  []*http.Response{httpResponse(http.StatusTooManyRequests, retryAfterHeader("2")), httpResponse(http.StatusOK, nil)},
			policy:     RetryPolicy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 5 * time.Second},
			wantStatus: http.StatusOK,
			wantDelays: []time.Duration{2 * time.Second},
		},
		{
			name:       "retry after is capped",
			responses:  []*http.Response{httpResponse(http.StatusTooManyRequests, retryAfterHeader("1000")), httpResponse(http.StatusOK, nil)},
			policy:     RetryPolicy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 5 * time.Second},
			wantStatus: http.StatusOK,
			wantDelays: []time.Duration{5 * time.Second},
		},
		{
			name:       "retry after HTTP date",
			responses:  []*http.Response{httpResponse(http.StatusTooManyRequests, retryAfterHeader(now.Add(3*time.Second).Format(http.TimeFormat))), httpResponse(http.StatusOK, nil)},
			policy:     RetryPolicy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 5 * time.Second},
			wantStatus: http.StatusOK,
			wantDelays: []time.Duration{3 * time.Second},
		},
		{
			name: "exponential backoff",
			responses: []*http.Response{
				httpResponse(http.StatusTooManyRequests, nil),
				httpResponse(http.StatusTooManyRequests, nil),
				httpResponse(http.StatusOK, nil),
			},
			policy:     RetryPolicy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 5 * time.Second},
			wantStatus: http.StatusOK,
			wantDelays: []time.Duration{100 * time.Millisecond, 200 * time.Millisecond},
		},
		{
			name:       "attempts are bounded",
			responses:  []*http.Response{httpResponse(http.StatusTooManyRequests, nil)},
			policy:     RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 10 * time.Millisecond},
			wantStatus: http.StatusTooManyRequests,
			wantDelays: []time.Duration{10 * time.Millisecond, 10 * time.Millisecond},
		},
		{
			name:       "server error is not retried",
			responses:  []*http.Response{httpResponse(http.StatusInternalServerError, nil)},
			policy:     RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 10 * time.Millisecond},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "service unavailable without retry after is not retried",
			responses:  []*http.Response{httpResponse(http.StatusServiceUnavailable, nil)},
			policy:     RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 10 * time.Millisecond},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "service unavailable with retry after is retried",
			responses:  []*http.Response{httpResponse(http.StatusServiceUnavailable, retryAfterHeader("1")), httpResponse(http.StatusOK, nil)},
			policy:     RetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 10 * time.Second},
			wantStatus: http.StatusOK,
			wantDelays: []time.Duration{time.Second},
		},
		{
			name:       "retries disabled",
			responses:  []*http.Response{httpResponse(http.StatusTooManyRequests, retryAfterHeader("1")), httpResponse(http.StatusOK, nil)},
			policy:     RetryPolicy{MaxAttempts: 1},
			wantStatus: http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://control.example.com/v2/tenants/acme", nil)
			if err != nil {
				t.Fatal(err)
			}

			var calls int
			var delays []time.Duration
			transport := &retryTransport{
				base: rtFunc(func(*http.Request) (*http.Response, error) {
					resp := tt.responses[min(calls, len(tt.responses)-1)]
					calls++
					return resp, nil
				}),
				policy: tt.policy,
				now:    func() time.Time { return now },
				sleep: func(_ context.Context, d time.Duration) error {
					delays = append(delays, d)
					return nil
				},
			}

			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip() error: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			wantCalls := len(tt.wantDelays) + 1
			if calls != wantCalls {
				t.Errorf("calls = %d, want %d", calls, wantCalls)
			}
			if len(delays) != len(tt.wantDelays) {
				t.Fatalf("delays = %v, want %v", delays, tt.wantDelays)
			}
			for i := range delays {
				if delays[i] != tt.wantDelays[i] {
					t.Errorf("delay[%d] = %v, want %v", i, delays[i], tt.wantDelays[i])
				}
			}
		})
	}
}

func TestRetryTransportReplaysBody(t *testing.T) {
	var bodies []string
	transport := &retryTransport{
		base: rtFunc(func(req *http.Request) (*http.Response, error) {
			data, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			bodies = append(bodies, string(data))
			if len(bodies) == 1 {
				return httpResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{"0"}}), nil
			}
			return httpResponse(http.StatusOK, nil), nil
		}),
		policy: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		now:    time.Now,
		sleep:  func(context.Context, time.Duration) error { return nil },
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://control.example.com/v2/tenants", strings.NewReader(`{"slug":"acme"}`))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() error: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("calls = %d, want 2", len(bodies))
	}
	if bodies[0] != `{"slug":"acme"}` || bodies[1] != `{"slug":"acme"}` {
		t.Errorf("bodies = %v, want the request body replayed", bodies)
	}
}

func TestRetryTransportDoesNotRetryUnreplayableBody(t *testing.T) {
	var calls int
	transport := &retryTransport{
		base: rtFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return httpResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{"0"}}), nil
		}),
		policy: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		now:    time.Now,
		sleep:  func(context.Context, time.Duration) error { return nil },
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://control.example.com/v2/tenants", io.NopCloser(strings.NewReader("body")))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = nil

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetryTransportContextCancellation(t *testing.T) {
	var calls int
	transport := &retryTransport{
		base: rtFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return httpResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{"60"}}), nil
		}),
		policy: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Minute},
		now:    time.Now,
		sleep:  sleepContext,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://control.example.com/v2/tenants/acme", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("RoundTrip() error = nil, want context cancellation error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{name: "empty", value: "", ok: false},
		{name: "invalid", value: "soon", ok: false},
		{name: "seconds", value: "15", want: 15 * time.Second, ok: true},
		{name: "negative seconds", value: "-4", want: 0, ok: true},
		{name: "date", value: now.Add(45 * time.Second).Format(http.TimeFormat), want: 45 * time.Second, ok: true},
		{name: "past date", value: now.Add(-time.Minute).Format(http.TimeFormat), want: 0, ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tt.value, now)
			if ok != tt.ok {
				t.Fatalf("parseRetryAfter() ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("parseRetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}
