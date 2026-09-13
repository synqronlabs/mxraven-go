package webhook_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

const (
	testSecret = "whsec_dGVzdHNlY3JldA"
	testKID    = "whk_test"
)

// signRequest reproduces the mxRaven signing algorithm independently of the
// package under test.
func signRequest(t *testing.T, secret string, req *http.Request, body []byte, timestamp, webhookID string) string {
	t.Helper()
	target := req.URL.EscapedPath()
	if target == "" {
		target = "/"
	}
	if req.URL.RawQuery != "" {
		target += "?" + req.URL.RawQuery
	}
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		timestamp,
		webhookID,
		req.Method,
		strings.ToLower(req.URL.Host),
		target,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(canonical)); err != nil {
		t.Fatalf("hash write: %v", err)
	}
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func signedRequest(t *testing.T, secret, body, timestamp string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://hooks.example.com/mxraven?tenant=acme", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(webhook.HeaderWebhookID, "task-123")
	req.Header.Set(webhook.HeaderTimestamp, timestamp)
	req.Header.Set(webhook.HeaderSignatureKID, testKID)
	req.Header.Set(webhook.HeaderSignature, signRequest(t, secret, req, []byte(body), timestamp, "task-123"))
	return req
}

const inboundBody = `{
	"event_type": "inbound_email",
	"task_id": "task-123",
	"tenant_id": "tenant-1",
	"listener_id": "listener-1",
	"attempt": 1,
	"accepted_at_utc": 1789302600,
	"occurred_at_utc": 1789302601,
	"routing_decision": {"terminal_action": "TERMINAL_ACTION_TYPE_RELAY", "used_listener_default": false},
	"envelope": {"mail_from": "sender@example.net", "rcpt_to": ["support@example.com"]},
	"message": {"subject": "Hello"},
	"headers": [{"name": "From", "value": "sender@example.net"}],
	"raw_email": {"url": "https://raw.example.com/messages/task-123.eml", "token_type": "Bearer", "access_token": "token"}
}`

func TestVerifierVerifyAndDecode(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := signedRequest(t, testSecret, inboundBody, timestamp)

	verifier, err := webhook.NewVerifier(webhook.WithSecret(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}

	event, err := verifier.VerifyAndDecode(req)
	if err != nil {
		t.Fatalf("VerifyAndDecode() error = %v", err)
	}
	if event.Type != webhook.EventTypeInboundEmail {
		t.Fatalf("Type = %q, want %q", event.Type, webhook.EventTypeInboundEmail)
	}
	if event.InboundEmail == nil {
		t.Fatal("InboundEmail = nil")
	}
	if event.InboundEmail.TaskID != "task-123" {
		t.Errorf("TaskID = %q", event.InboundEmail.TaskID)
	}
	if event.InboundEmail.RoutingDecision.TerminalAction != webhook.TerminalActionRelay {
		t.Errorf("TerminalAction = %q", event.InboundEmail.RoutingDecision.TerminalAction)
	}
}

func TestVerifierRestoresBody(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := signedRequest(t, testSecret, inboundBody, timestamp)

	verifier, err := webhook.NewVerifier(webhook.WithSecret(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := verifier.Verify(req); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if string(body) != inboundBody {
		t.Errorf("restored body = %q", body)
	}
}

func TestVerifierRejectsTamperedBody(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := signedRequest(t, testSecret, inboundBody, timestamp)
	req.Body = io.NopCloser(strings.NewReader(inboundBody + " "))

	verifier, err := webhook.NewVerifier(webhook.WithSecret(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := verifier.Verify(req); !errors.Is(err, webhook.ErrInvalidSignature) {
		t.Fatalf("Verify() error = %v, want ErrInvalidSignature", err)
	}
}

func TestVerifierRejectsWrongSecret(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := signedRequest(t, "whsec_other", inboundBody, timestamp)

	verifier, err := webhook.NewVerifier(webhook.WithSecret(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := verifier.Verify(req); !errors.Is(err, webhook.ErrInvalidSignature) {
		t.Fatalf("Verify() error = %v, want ErrInvalidSignature", err)
	}
}

func TestVerifierMissingHeaders(t *testing.T) {
	headers := []string{
		webhook.HeaderWebhookID,
		webhook.HeaderTimestamp,
		webhook.HeaderSignature,
	}
	for _, header := range headers {
		t.Run(header, func(t *testing.T) {
			timestamp := strconv.FormatInt(time.Now().Unix(), 10)
			req := signedRequest(t, testSecret, inboundBody, timestamp)
			req.Header.Del(header)

			verifier, err := webhook.NewVerifier(webhook.WithSecret(testSecret))
			if err != nil {
				t.Fatalf("NewVerifier() error = %v", err)
			}
			if err := verifier.Verify(req); err == nil {
				t.Fatalf("Verify() error = nil, want error for missing %s", header)
			}
		})
	}
}

func TestVerifierTolerance(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	req := signedRequest(t, testSecret, inboundBody, timestamp)

	strict, err := webhook.NewVerifier(webhook.WithSecret(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := strict.Verify(req); err == nil {
		t.Fatal("Verify() error = nil, want stale timestamp error")
	}

	relaxed, err := webhook.NewVerifier(
		webhook.WithSecret(testSecret),
		webhook.WithTolerance(0),
	)
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := relaxed.Verify(req); err != nil {
		t.Fatalf("Verify() with disabled tolerance error = %v", err)
	}
}

func TestVerifierWithKey(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := signedRequest(t, testSecret, inboundBody, timestamp)

	verifier, err := webhook.NewVerifier(webhook.WithKey(testKID, testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := verifier.Verify(req); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	unknown, err := webhook.NewVerifier(webhook.WithKey("whk_other", testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := unknown.Verify(req); err == nil {
		t.Fatal("Verify() with unknown key ID error = nil, want error")
	}

	req.Header.Del(webhook.HeaderSignatureKID)
	if err := verifier.Verify(req); err == nil {
		t.Fatal("Verify() with missing key ID error = nil, want error")
	}
}

func TestVerifierMaxBody(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := signedRequest(t, testSecret, inboundBody, timestamp)

	verifier, err := webhook.NewVerifier(
		webhook.WithSecret(testSecret),
		webhook.WithMaxBodyBytes(16),
	)
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := verifier.Verify(req); err == nil {
		t.Fatal("Verify() error = nil, want body size error")
	}
}

func TestNewVerifierRequiresSecret(t *testing.T) {
	if _, err := webhook.NewVerifier(); err == nil {
		t.Fatal("NewVerifier() error = nil, want error")
	}
	if _, err := webhook.NewVerifier(webhook.WithSecret("")); err == nil {
		t.Fatal("NewVerifier(WithSecret(\"\")) error = nil, want error")
	}
}

func TestNewVerifierRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  webhook.VerifierOption
	}{
		{"empty key id", webhook.WithKey("", testSecret)},
		{"empty key secret", webhook.WithKey(testKID, "")},
		{"negative tolerance", webhook.WithTolerance(-time.Second)},
		{"non-positive max body", webhook.WithMaxBodyBytes(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := webhook.NewVerifier(tt.opt); err == nil {
				t.Fatal("NewVerifier() error = nil, want error")
			}
		})
	}
}

func TestVerifierRejectsUnsupportedScheme(t *testing.T) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := signedRequest(t, testSecret, inboundBody, timestamp)
	req.Header.Set(webhook.HeaderSignature, "sha1=deadbeef")

	verifier, err := webhook.NewVerifier(webhook.WithSecret(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := verifier.Verify(req); err == nil {
		t.Fatal("Verify() error = nil, want error")
	}
}

func TestVerifierUsesRequestTarget(t *testing.T) {
	// A signature over a different path must not verify.
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := signedRequest(t, testSecret, inboundBody, timestamp)
	req.URL.Path = "/different"

	verifier, err := webhook.NewVerifier(webhook.WithSecret(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if err := verifier.Verify(req); !errors.Is(err, webhook.ErrInvalidSignature) {
		t.Fatalf("Verify() error = %v, want ErrInvalidSignature", err)
	}
}
