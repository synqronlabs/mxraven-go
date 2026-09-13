package mail

import (
	"crypto/tls"
	"strings"
	"testing"
	"time"
)

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		wantErr bool
	}{
		{
			name: "valid",
			opts: []Option{
				WithAddress("smtp.example.com", 587),
				WithCredentials("mxr_tx_ab12cd34ef56", "secret"),
			},
		},
		{
			name:    "missing address",
			opts:    []Option{WithCredentials("user", "secret")},
			wantErr: true,
		},
		{
			name:    "missing credentials",
			opts:    []Option{WithAddress("smtp.example.com", 587)},
			wantErr: true,
		},
		{
			name: "empty username",
			opts: []Option{
				WithAddress("smtp.example.com", 587),
				WithCredentials("", "secret"),
			},
			wantErr: true,
		},
		{
			name: "empty secret",
			opts: []Option{
				WithAddress("smtp.example.com", 587),
				WithCredentials("user", ""),
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			opts: []Option{
				WithAddress("smtp.example.com", 0),
				WithCredentials("user", "secret"),
			},
			wantErr: true,
		},
		{
			name: "invalid pool size",
			opts: []Option{
				WithAddress("smtp.example.com", 587),
				WithCredentials("user", "secret"),
				WithPoolSize(-1),
			},
			wantErr: true,
		},
		{
			name: "invalid timeout",
			opts: []Option{
				WithAddress("smtp.example.com", 587),
				WithCredentials("user", "secret"),
				WithConnectTimeout(0),
			},
			wantErr: true,
		},
		{
			name: "nil TLS config",
			opts: []Option{
				WithAddress("smtp.example.com", 587),
				WithCredentials("user", "secret"),
				WithTLSConfig(nil),
			},
			wantErr: true,
		},
		{
			name: "custom TLS config",
			opts: []Option{
				WithAddress("smtp.example.com", 587),
				WithCredentials("user", "secret"),
				WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS13}),
				WithPoolSize(2),
				WithConnectTimeout(5 * time.Second),
				WithLocalName("mailer.example.com"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := New(tt.opts...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("New() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if client == nil {
				t.Fatal("New() client = nil")
			}
			if err := client.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
	}
}

func TestSendRequiresMessage(t *testing.T) {
	client, err := New(
		WithAddress("smtp.example.com", 587),
		WithCredentials("user", "secret"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	if _, err := client.Send(t.Context(), nil); err == nil {
		t.Fatal("Send(nil) error = nil, want error")
	}
	if _, err := client.SendRaw(t.Context(), Envelope{To: []string{"a@example.com"}}, nil); err == nil {
		t.Fatal("SendRaw(nil) error = nil, want error")
	}
}

func TestEnvelopeRaven(t *testing.T) {
	tests := []struct {
		name     string
		envelope Envelope
		wantErr  bool
	}{
		{
			name:     "valid",
			envelope: Envelope{From: "sender@example.com", To: []string{"a@example.com", "b@example.com"}},
		},
		{
			name:     "null sender",
			envelope: Envelope{To: []string{"a@example.com"}},
		},
		{
			name:     "no recipients",
			envelope: Envelope{From: "sender@example.com"},
			wantErr:  true,
		},
		{
			name:     "invalid sender",
			envelope: Envelope{From: "not an address", To: []string{"a@example.com"}},
			wantErr:  true,
		},
		{
			name:     "invalid recipient",
			envelope: Envelope{From: "sender@example.com", To: []string{"not an address"}},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope, err := tt.envelope.raven()
			if tt.wantErr {
				if err == nil {
					t.Fatal("raven() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("raven() error = %v", err)
			}
			if len(envelope.To) != len(tt.envelope.To) {
				t.Errorf("len(To) = %d, want %d", len(envelope.To), len(tt.envelope.To))
			}
			if tt.envelope.From == "" && !envelope.From.IsNull() {
				t.Error("expected null reverse-path")
			}
		})
	}
}

func TestSMTPError(t *testing.T) {
	permanent := &SMTPError{Code: 550, EnhancedCode: "5.7.1", Message: "not authorized"}
	if !permanent.Permanent() || permanent.Transient() {
		t.Error("550 error classification is wrong")
	}
	if !strings.Contains(permanent.Error(), "5.7.1") {
		t.Errorf("Error() = %q", permanent.Error())
	}

	transient := &SMTPError{Code: 451, Message: "try later"}
	if transient.Permanent() || !transient.Transient() {
		t.Error("451 error classification is wrong")
	}
}
