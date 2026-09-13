package mail

import (
	"strings"
	"testing"
	"time"
)

func TestMessageBuildHeaders(t *testing.T) {
	built, err := NewMessage().
		From("Acme <noreply@acme.example>").
		To("customer@example.com").
		Cc("ops@example.com").
		Subject("Your receipt").
		Text("Thanks for your order.").
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}

	headers := built.Content.Headers
	for name, want := range map[string]string{
		"From":    "Acme <noreply@acme.example>",
		"To":      "customer@example.com",
		"Cc":      "ops@example.com",
		"Subject": "Your receipt",
	} {
		if got := headers.Get(name); got != want {
			t.Errorf("header %s = %q, want %q", name, got, want)
		}
	}
	if headers.Get("Message-ID") == "" {
		t.Error("Message-ID header is empty")
	}
	if headers.Get("Date") == "" {
		t.Error("Date header is empty")
	}
}

func TestMessageBuildRejectsInvalidAddress(t *testing.T) {
	if _, err := NewMessage().From("not an address").To("a@example.com").build(); err == nil {
		t.Fatal("build() error = nil, want error")
	}
	if _, err := NewMessage().From("a@example.com").To("not an address").build(); err == nil {
		t.Fatal("build() error = nil, want error")
	}
	if _, err := NewMessage().To("a@example.com").build(); err == nil {
		t.Fatal("build() with no from error = nil, want error")
	}
}

func TestMessageBuildAlternative(t *testing.T) {
	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		Text("plain body").
		HTML("<p>html body</p>").
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}

	contentType := built.Content.Headers.Get("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/alternative") {
		t.Fatalf("Content-Type = %q, want multipart/alternative", contentType)
	}
	body := string(built.Content.Body)
	if !strings.Contains(body, "plain body") {
		t.Errorf("body does not contain plain text part:\n%s", body)
	}
	if !strings.Contains(body, "<p>html body</p>") {
		t.Errorf("body does not contain HTML part:\n%s", body)
	}
	if !strings.Contains(body, "text/plain") || !strings.Contains(body, "text/html") {
		t.Errorf("body does not declare both alternative parts:\n%s", body)
	}
}

func TestMessageBuildAlternativeNonASCII(t *testing.T) {
	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		Text("grüße").
		HTML("<p>grüße</p>").
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	if got := built.Content.Headers.Get("Content-Transfer-Encoding"); got != "8bit" {
		t.Errorf("Content-Transfer-Encoding = %q, want 8bit", got)
	}
}

func TestMessageBuildAttachments(t *testing.T) {
	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		Text("hello").
		AttachFile("notes.txt", []byte("attached")).
		AttachInline("logo.png", "logo", []byte("png")).
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}

	contentType := built.Content.Headers.Get("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/mixed") {
		t.Fatalf("Content-Type = %q, want multipart/mixed", contentType)
	}
	body := string(built.Content.Body)
	// Attachment data is base64-encoded by the MIME writer.
	for _, want := range []string{"hello", "notes.txt", "YXR0YWNoZWQ=", "logo.png", "inline", "cG5n"} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q:\n%s", want, body)
		}
	}
}

func TestMessageBuildNullSender(t *testing.T) {
	built, err := NewMessage().
		From("Mailer Daemon <mailer-daemon@example.com>").
		NullSender().
		To("b@example.com").
		Text("bounce").
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	if !built.Envelope.From.IsNull() {
		t.Error("envelope sender is not null")
	}
	if got := built.Content.Headers.Get("From"); got != "Mailer Daemon <mailer-daemon@example.com>" {
		t.Errorf("From header = %q", got)
	}
}

func TestMessageBuildCustomHeaderAndDate(t *testing.T) {
	date := time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)
	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		Header("X-Campaign", "launch").
		Date(date).
		Text("hello").
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	if got := built.Content.Headers.Get("X-Campaign"); got != "launch" {
		t.Errorf("X-Campaign = %q", got)
	}
	if got := built.Content.Headers.Get("Date"); !strings.Contains(got, "2026") {
		t.Errorf("Date = %q, want 2026 date", got)
	}
}

func TestNormalizeCRLF(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lf", "a\nb", "a\r\nb"},
		{"crlf", "a\r\nb", "a\r\nb"},
		{"cr", "a\rb", "a\r\nb"},
		{"mixed", "a\r\nb\nc\rd", "a\r\nb\r\nc\r\nd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeCRLF(tt.in); got != tt.want {
				t.Errorf("normalizeCRLF(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestContainsNonASCII(t *testing.T) {
	if containsNonASCII("plain ascii") {
		t.Error("containsNonASCII(plain) = true")
	}
	if !containsNonASCII("grüße") {
		t.Error("containsNonASCII(grüße) = false")
	}
}
