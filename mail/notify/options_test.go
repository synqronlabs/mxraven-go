package notify

import (
	"crypto/tls"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestNewValidatesOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{name: "no URL", want: "URL is required"},
		{name: "bad scheme", opts: []Option{WithURL("nats://nats.example.com")}, want: "must use the ws or wss scheme"},
		{name: "no credentials", opts: []Option{WithURL("wss://nats.example.com")}, want: "credentials are required"},
		{
			name: "no listener",
			opts: []Option{WithURL("wss://nats.example.com"), WithCredentials("key", "secret")},
			want: "listener ID is required",
		},
		{
			name: "invalid listener token",
			opts: []Option{WithURL("wss://nats.example.com"), WithCredentials("key", "secret"), WithListener("bad.listener")},
			want: "listener ID",
		},
		{
			name: "invalid consumer token",
			opts: []Option{WithURL("wss://nats.example.com"), WithCredentials("key", "secret"), WithListener("listener-1"), WithConsumer("bad.consumer")},
			want: "consumer name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts...)
			if err == nil {
				t.Fatal("New() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestOptionValidation(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
	}{
		{name: "empty URL", opt: WithURL("")},
		{name: "empty username", opt: WithCredentials("", "secret")},
		{name: "empty secret", opt: WithCredentials("key", "")},
		{name: "empty listener", opt: WithListener("")},
		{name: "nil TLS config", opt: WithTLSConfig(nil)},
		{name: "zero timeout", opt: WithConnectTimeout(0)},
		{name: "nil error handler", opt: WithErrorHandler(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{timeout: defaultConnectTimeout}
			if err := tt.opt(&cfg); err == nil {
				t.Fatal("option error = nil, want error")
			}
		})
	}
}

func TestWithConsumerOverride(t *testing.T) {
	cfg := config{timeout: defaultConnectTimeout}
	if err := WithConsumer("notify-custom")(&cfg); err != nil {
		t.Fatalf("WithConsumer() error = %v", err)
	}
	if cfg.consumer != "notify-custom" {
		t.Fatalf("consumer = %q, want %q", cfg.consumer, "notify-custom")
	}
}

func TestValidToken(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "listener-1", want: true},
		{value: "0b0b0b0b-1111-2222-3333-444444444444", want: true},
		{value: "", want: false},
		{value: "a.b", want: false},
		{value: "a*b", want: false},
		{value: "a>b", want: false},
		{value: "a b", want: false},
	}
	for _, tt := range tests {
		if got := validToken(tt.value); got != tt.want {
			t.Errorf("validToken(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestIsWebSocketURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "ws://localhost:8080", want: true},
		{url: "WSS://nats.example.com", want: true},
		{url: "wss://nats.example.com", want: true},
		{url: "nats://nats.example.com", want: false},
		{url: "tls://nats.example.com", want: false},
		{url: "https://nats.example.com", want: false},
		{url: "", want: false},
	}
	for _, tt := range tests {
		if got := isWebSocketURL(tt.url); got != tt.want {
			t.Errorf("isWebSocketURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestDefaultConsumer(t *testing.T) {
	if got, want := defaultConsumer("listener-1"), "notify-listener-1"; got != want {
		t.Errorf("defaultConsumer() = %q, want %q", got, want)
	}
}

func TestClientCloseIsIdempotent(t *testing.T) {
	client := &Client{done: make(chan struct{})}
	closed := 0
	client.closeInit = func() { closed++ }

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() second error = %v", err)
	}
	if closed != 1 {
		t.Fatalf("close function called %d times, want 1", closed)
	}
}

func TestClientCloseNil(t *testing.T) {
	var client *Client
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestRetryDelayCaps(t *testing.T) {
	msg := &fakeMsg{delivered: 100}
	if got := retryDelay(msg); got != defaultRetryMax {
		t.Fatalf("retryDelay() = %v, want %v", got, defaultRetryMax)
	}
}

func TestWithConnectTimeout(t *testing.T) {
	cfg := config{}
	if err := WithConnectTimeout(3 * time.Second)(&cfg); err != nil {
		t.Fatalf("WithConnectTimeout() error = %v", err)
	}
	if cfg.timeout != 3*time.Second {
		t.Fatalf("timeout = %v, want %v", cfg.timeout, 3*time.Second)
	}
}

func TestWithTLSConfig(t *testing.T) {
	cfg := config{}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if err := WithTLSConfig(tlsConfig)(&cfg); err != nil {
		t.Fatalf("WithTLSConfig() error = %v", err)
	}
	if cfg.tlsConfig != tlsConfig {
		t.Fatal("TLS config was not stored")
	}
}

func TestNATSOptions(t *testing.T) {
	cfg := config{username: "key-prefix", secret: "key-secret", listenerID: "listener-1", timeout: 5 * time.Second}
	options := nats.Options{}
	for _, apply := range natsOptions(cfg) {
		if err := apply(&options); err != nil {
			t.Fatalf("nats option error = %v", err)
		}
	}
	if options.User != "key-prefix" || options.Password != "key-secret" {
		t.Errorf("credentials = %q/%q, want key-prefix/key-secret", options.User, options.Password)
	}
	if got, want := options.InboxPrefix, "_INBOX.listener-1"; got != want {
		t.Errorf("InboxPrefix = %q, want %q", got, want)
	}
	if got, want := options.MaxReconnect, -1; got != want {
		t.Errorf("MaxReconnect = %d, want %d", got, want)
	}
	if got, want := options.Timeout, 5*time.Second; got != want {
		t.Errorf("Timeout = %v, want %v", got, want)
	}
}
