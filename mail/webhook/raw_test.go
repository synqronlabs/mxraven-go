package webhook_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

const rawMessage = "From: sender@example.net\r\nSubject: Hello\r\n\r\nbody\r\n"

func rawEmailPayload(serverURL, digest string, size int64) webhook.RawEmail {
	return webhook.RawEmail{
		URL:          serverURL + "/messages/task-1.eml",
		TokenType:    "Bearer",
		AccessToken:  "short-lived-token",
		ExpiresAtUTC: 4102444800,
		SizeBytes:    size,
		SHA256Hex:    digest,
	}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func TestRawEmailFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer short-lived-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "message/rfc822")
		_, _ = w.Write([]byte(rawMessage))
	}))
	defer server.Close()

	payload := rawEmailPayload(server.URL, sha256Hex(rawMessage), int64(len(rawMessage)))
	body, err := payload.Fetch(context.Background(), server.Client())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(body) != rawMessage {
		t.Errorf("body = %q", body)
	}
}

func TestRawEmailFetchDefaultTokenType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer short-lived-token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(rawMessage))
	}))
	defer server.Close()

	payload := webhook.RawEmail{
		URL:         server.URL + "/messages/task-1.eml",
		AccessToken: "short-lived-token",
	}
	if _, err := payload.Fetch(context.Background(), server.Client()); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
}

func TestRawEmailFetchIntegrityFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rawMessage))
	}))
	defer server.Close()

	tests := []struct {
		name    string
		payload webhook.RawEmail
	}{
		{
			name:    "digest mismatch",
			payload: rawEmailPayload(server.URL, sha256Hex("something else"), int64(len(rawMessage))),
		},
		{
			name:    "size mismatch",
			payload: rawEmailPayload(server.URL, sha256Hex(rawMessage), int64(len(rawMessage))+10),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.payload.Fetch(context.Background(), server.Client()); err == nil {
				t.Fatal("Fetch() error = nil, want integrity error")
			}
		})
	}
}

func TestRawEmailFetchStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	payload := webhook.RawEmail{
		URL:         server.URL + "/messages/task-1.eml",
		AccessToken: "expired",
	}
	_, err := payload.Fetch(context.Background(), server.Client())
	if err == nil {
		t.Fatal("Fetch() error = nil, want status error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want status 401", err)
	}
}

func TestRawEmailFetchMissingFields(t *testing.T) {
	if _, err := (webhook.RawEmail{AccessToken: "token"}).Fetch(context.Background(), nil); err == nil {
		t.Fatal("Fetch() with missing URL error = nil, want error")
	}
	if _, err := (webhook.RawEmail{URL: "https://example.com/messages/x.eml"}).Fetch(context.Background(), nil); err == nil {
		t.Fatal("Fetch() with missing token error = nil, want error")
	}
}
