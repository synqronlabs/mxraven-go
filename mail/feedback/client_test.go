package feedback_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/synqronlabs/mxraven-go/mail/feedback"
)

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    []feedback.Option
		wantErr bool
	}{
		{name: "valid", opts: []feedback.Option{feedback.WithBaseURL("https://feedback.example.com")}},
		{name: "missing base URL", wantErr: true},
		{name: "empty base URL", opts: []feedback.Option{feedback.WithBaseURL("")}, wantErr: true},
		{
			name: "empty username",
			opts: []feedback.Option{
				feedback.WithBaseURL("https://feedback.example.com"),
				feedback.WithCredentials("", "secret"),
			},
			wantErr: true,
		},
		{
			name: "empty secret",
			opts: []feedback.Option{
				feedback.WithBaseURL("https://feedback.example.com"),
				feedback.WithCredentials("user", ""),
			},
			wantErr: true,
		},
		{
			name: "nil HTTP client",
			opts: []feedback.Option{
				feedback.WithBaseURL("https://feedback.example.com"),
				feedback.WithHTTPClient(nil),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := feedback.New(tt.opts...)
			if tt.wantErr && err == nil {
				t.Fatal("New() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("New() error = %v", err)
			}
		})
	}
}

func TestLearnSpam(t *testing.T) {
	const (
		username = "mxr_tx_ab12cd34ef56"
		secret   = "supersecret"
		rawMIME  = "From: a@example.com\r\nSubject: sample\r\n\r\nbody\r\n"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/feedback/learn/spam" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "message/rfc822" {
			t.Errorf("Content-Type = %q", got)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != secret {
			t.Errorf("basic auth = %q/%q/%v", user, pass, ok)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != rawMIME {
			t.Errorf("body = %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"learned","disposition":"spam","tenant_id":"t1","listener_id":"l1","matched_hash_kind":"rendered_eml_sha256"}`)
	}))
	defer server.Close()

	client, err := feedback.New(
		feedback.WithBaseURL(server.URL),
		feedback.WithCredentials(username, secret),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := client.LearnSpam(context.Background(), strings.NewReader(rawMIME))
	if err != nil {
		t.Fatalf("LearnSpam() error = %v", err)
	}
	if result.Status != "learned" {
		t.Errorf("Status = %q", result.Status)
	}
	if result.Disposition != feedback.DispositionSpam {
		t.Errorf("Disposition = %q", result.Disposition)
	}
	if result.ListenerID != "l1" {
		t.Errorf("ListenerID = %q", result.ListenerID)
	}
}

func TestLearnHam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/feedback/learn/ham" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"status":"learned","disposition":"ham","tenant_id":"t1","listener_id":"l1","matched_hash_kind":"accepted_eml_sha256"}`)
	}))
	defer server.Close()

	client, err := feedback.New(
		feedback.WithBaseURL(server.URL),
		feedback.WithCredentials("user", "secret"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.LearnHam(context.Background(), strings.NewReader("message")); err != nil {
		t.Fatalf("LearnHam() error = %v", err)
	}
}

func TestLearnErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"message evidence not found"}`)
	}))
	defer server.Close()

	client, err := feedback.New(
		feedback.WithBaseURL(server.URL),
		feedback.WithCredentials("user", "secret"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.LearnSpam(context.Background(), strings.NewReader("message"))
	var httpErr *feedback.Error
	if !errors.As(err, &httpErr) {
		t.Fatalf("LearnSpam() error = %T, want *feedback.Error", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d", httpErr.StatusCode)
	}
	if httpErr.Message != "message evidence not found" {
		t.Errorf("Message = %q", httpErr.Message)
	}
	if httpErr.Retryable() {
		t.Error("Retryable() = true for 404")
	}
}

func TestLearnRequiresCredentials(t *testing.T) {
	client, err := feedback.New(feedback.WithBaseURL("https://feedback.example.com"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.LearnSpam(context.Background(), strings.NewReader("message")); err == nil {
		t.Fatal("LearnSpam() error = nil, want credentials error")
	}
	if _, err := client.Learn(context.Background(), "bogus", strings.NewReader("message")); err == nil {
		t.Fatal("Learn() error = nil, want disposition error")
	}
	if _, err := client.Learn(context.Background(), feedback.DispositionSpam, nil); err == nil {
		t.Fatal("Learn(nil) error = nil, want error")
	}
}

func TestRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"learning rate limit exceeded"}`)
	}))
	defer server.Close()

	client, err := feedback.New(feedback.WithBaseURL(server.URL), feedback.WithCredentials("user", "secret"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.LearnSpam(context.Background(), strings.NewReader("message"))
	var httpErr *feedback.Error
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T, want *feedback.Error", err)
	}
	if !httpErr.Retryable() {
		t.Error("Retryable() = false for 429")
	}
}

func TestUnsubscribe(t *testing.T) {
	const token = "header.payload.signature"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if want := "/v1/feedback/unsubscribe/" + token; r.URL.Path != want {
			t.Errorf("path = %s, want %s", r.URL.Path, want)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "List-Unsubscribe=One-Click" {
			t.Errorf("body = %q", body)
		}
		_, _ = io.WriteString(w, `{"status":"unsubscribed"}`)
	}))
	defer server.Close()

	client, err := feedback.New(feedback.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := client.Unsubscribe(context.Background(), token); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}
	if err := client.Unsubscribe(context.Background(), "  "); err == nil {
		t.Fatal("Unsubscribe() with empty token error = nil, want error")
	}
}
