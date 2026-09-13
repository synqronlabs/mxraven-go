package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/synqronlabs/mxraven-go/admin"
	"github.com/synqronlabs/mxraven-go/admin/api"
)

const testProblemJSON = `{
	"type": "urn:mxraven:problem:not_found",
	"title": "Not Found",
	"status": 404,
	"code": "not_found"
}`

const testTenantJSON = `{
	"id": "11111111-1111-1111-1111-111111111111",
	"slug": "acme",
	"status": "active",
	"provisioning_state": "ready",
	"desired_generation": 1,
	"applied_generation": 1
}`

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    []admin.Option
		wantErr bool
	}{
		{
			name:    "missing base URL and auth",
			wantErr: true,
		},
		{
			name:    "missing auth",
			opts:    []admin.Option{admin.WithBaseURL("https://control.example.com")},
			wantErr: true,
		},
		{
			name:    "missing base URL",
			opts:    []admin.Option{admin.WithToken("secret")},
			wantErr: true,
		},
		{
			name:    "empty base URL",
			opts:    []admin.Option{admin.WithBaseURL(""), admin.WithToken("secret")},
			wantErr: true,
		},
		{
			name:    "empty token",
			opts:    []admin.Option{admin.WithBaseURL("https://control.example.com"), admin.WithToken("")},
			wantErr: true,
		},
		{
			name:    "nil token source",
			opts:    []admin.Option{admin.WithBaseURL("https://control.example.com"), admin.WithTokenSource(nil)},
			wantErr: true,
		},
		{
			name:    "nil HTTP client",
			opts:    []admin.Option{admin.WithBaseURL("https://control.example.com"), admin.WithToken("secret"), admin.WithHTTPClient(nil)},
			wantErr: true,
		},
		{
			name: "valid",
			opts: []admin.Option{admin.WithBaseURL("https://control.example.com"), admin.WithToken("secret")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := admin.New(tt.opts...)
			if tt.wantErr && err == nil {
				t.Fatal("New() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}
		})
	}
}

func TestBearerToken(t *testing.T) {
	const traceID = "0123456789abcdef0123456789abcdef"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/v2/tenants/acme"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Trace-ID", traceID)
		_, _ = w.Write([]byte(testTenantJSON))
	}))
	t.Cleanup(srv.Close)

	client, err := admin.New(admin.WithBaseURL(srv.URL), admin.WithToken("secret"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	res, err := client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err != nil {
		t.Fatalf("GetTenant() error: %v", err)
	}

	ok, isOK := res.(*api.TenantHeaders)
	if !isOK {
		t.Fatalf("GetTenant() response type = %T, want *api.TenantHeaders", res)
	}
	if got, want := string(ok.Response.Slug), "acme"; got != want {
		t.Errorf("tenant slug = %q, want %q", got, want)
	}
	if got, want := ok.XTraceID.Value, traceID; got != want {
		t.Errorf("X-Trace-ID = %q, want %q", got, want)
	}

	trace, found := admin.TraceID(res)
	if !found || trace != traceID {
		t.Errorf("TraceID() = (%q, %v), want (%q, true)", trace, found, traceID)
	}
}

func TestTokenSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer dynamic"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testTenantJSON))
	}))
	t.Cleanup(srv.Close)

	var operation string
	client, err := admin.New(
		admin.WithBaseURL(srv.URL),
		admin.WithTokenSource(func(_ context.Context, op string) (string, error) {
			operation = op
			return "dynamic", nil
		}),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if _, err := client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"}); err != nil {
		t.Fatalf("GetTenant() error: %v", err)
	}
	if want := string(api.GetTenantOperation); operation != want {
		t.Errorf("token source operation = %q, want %q", operation, want)
	}
}

func TestTokenSourceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached the server despite a token source error")
	}))
	t.Cleanup(srv.Close)

	wantErr := errors.New("credentials unavailable")
	client, err := admin.New(
		admin.WithBaseURL(srv.URL),
		admin.WithTokenSource(func(context.Context, string) (string, error) {
			return "", wantErr
		}),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	_, err = client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err == nil {
		t.Fatal("GetTenant() error = nil, want error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("GetTenant() error = %v, want chain containing %v", err, wantErr)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestWithHTTPClientAndUserAgent(t *testing.T) {
	var called bool
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		if got, want := r.Header.Get("User-Agent"), "mxraven-go-test"; got != want {
			t.Errorf("User-Agent = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       http.NoBody,
			Request:    r,
		}, nil
	})

	client, err := admin.New(
		admin.WithBaseURL("https://control.example.com"),
		admin.WithToken("secret"),
		admin.WithUserAgent("mxraven-go-test"),
		admin.WithHTTPClient(&http.Client{Transport: transport}),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	_, err = client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err == nil {
		t.Fatal("GetTenant() error = nil, want decode error from empty body")
	}
	if !called {
		t.Fatal("custom HTTP client transport was not used")
	}
}

func TestRetryIntegration(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(testProblemJSON))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testTenantJSON))
	}))
	t.Cleanup(srv.Close)

	client, err := admin.New(admin.WithBaseURL(srv.URL), admin.WithToken("secret"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	res, err := client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err != nil {
		t.Fatalf("GetTenant() error: %v", err)
	}
	if _, ok := res.(*api.TenantHeaders); !ok {
		t.Fatalf("GetTenant() response type = %T, want *api.TenantHeaders", res)
	}
	if requests != 2 {
		t.Errorf("requests = %d, want 2", requests)
	}
}

func TestRetryDisabled(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Retry-After", "0")
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(testProblemJSON))
	}))
	t.Cleanup(srv.Close)

	client, err := admin.New(
		admin.WithBaseURL(srv.URL),
		admin.WithToken("secret"),
		admin.WithRetry(admin.RetryPolicy{MaxAttempts: 1}),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	res, err := client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err != nil {
		t.Fatalf("GetTenant() error: %v", err)
	}
	if _, ok := admin.ProblemFrom(res); !ok {
		t.Fatalf("GetTenant() response type = %T, want a problem response", res)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
}

func TestProblemFrom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(testProblemJSON))
	}))
	t.Cleanup(srv.Close)

	client, err := admin.New(admin.WithBaseURL(srv.URL), admin.WithToken("secret"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	res, err := client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err != nil {
		t.Fatalf("GetTenant() transport error: %v", err)
	}

	got, ok := admin.ProblemFrom(res)
	if !ok {
		t.Fatalf("ProblemFrom(%T) = false, want true", res)
	}
	if got.Code != "not_found" || got.Status != http.StatusNotFound {
		t.Errorf("ProblemFrom() = %+v, want code %q status %d", got, "not_found", http.StatusNotFound)
	}

	if _, ok := admin.ProblemFrom(&api.TenantHeaders{}); ok {
		t.Error("ProblemFrom(success response) = true, want false")
	}
	if _, ok := admin.ProblemFrom(struct{}{}); ok {
		t.Error("ProblemFrom(unknown type) = true, want false")
	}
}
