package mime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

func TestFetchAndParseRawEmail(t *testing.T) {
	message := []byte("Subject: Inbound\r\nContent-Type: text/plain\r\n\r\nhello")
	sum := sha256.Sum256(message)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer token"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(message)
	}))
	defer server.Close()

	email, err := FetchAndParseRawEmail(context.Background(), webhook.RawEmail{
		URL:         server.URL,
		TokenType:   "Bearer",
		AccessToken: "token",
		SizeBytes:   int64(len(message)),
		SHA256Hex:   hex.EncodeToString(sum[:]),
	}, server.Client())
	if err != nil {
		t.Fatalf("FetchAndParseRawEmail() error = %v", err)
	}
	if got, want := email.Subject, "Inbound"; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if got, want := email.Text, "hello"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

func TestFetchAndParseRawEmailDigestMismatch(t *testing.T) {
	message := []byte("Subject: Inbound\r\nContent-Type: text/plain\r\n\r\nhello")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(message)
	}))
	defer server.Close()

	_, err := FetchAndParseRawEmail(context.Background(), webhook.RawEmail{
		URL:         server.URL,
		AccessToken: "token",
		SizeBytes:   int64(len(message)),
		SHA256Hex:   hex.EncodeToString(make([]byte, sha256.Size)),
	}, server.Client())
	if err == nil {
		t.Fatal("FetchAndParseRawEmail() error = nil, want digest mismatch")
	}
}
