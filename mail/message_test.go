package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
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

func TestLongestLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"single", "abc", 3},
		{"lf", "a\nbbb\ncc", 3},
		{"crlf", "a\r\nbbb\r\ncc", 3},
		{"bare cr", "a\rbbb\rcc", 3},
		{"no trailing newline", "aa\nbbbb", 4},
		{"trailing newline", "aa\nbbbb\n", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := longestLine(tt.in); got != tt.want {
				t.Errorf("longestLine(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestNeedsQuotedPrintable(t *testing.T) {
	if needsQuotedPrintable(strings.Repeat("a", 998)) {
		t.Error("998-char line should not need quoted-printable encoding")
	}
	if !needsQuotedPrintable(strings.Repeat("a", 999)) {
		t.Error("999-char line should need quoted-printable encoding")
	}
	if needsQuotedPrintable("short\r\nlines\r\n") {
		t.Error("short lines should not need quoted-printable encoding")
	}
}

func TestMessageBuildQuotedPrintableTextBody(t *testing.T) {
	long := "line one\r\n" + strings.Repeat("a", 1200) + "\r\nline three"

	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		Text(long).
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}

	if got := built.Content.Headers.Get("Content-Transfer-Encoding"); got != "quoted-printable" {
		t.Fatalf("Content-Transfer-Encoding = %q, want quoted-printable", got)
	}
	assertQuotedPrintableLines(t, built.Content.Body)
	if err := built.Content.Validate(); err != nil {
		t.Errorf("Content.Validate() error = %v, want nil", err)
	}
	if got := decodeQuotedPrintable(t, built.Content.Body); got != normalizeCRLF(long) {
		t.Errorf("decoded body = %q, want %q", got, normalizeCRLF(long))
	}
}

func TestMessageBuildQuotedPrintableHTMLBody(t *testing.T) {
	long := "<p>" + strings.Repeat("x", 1500) + "</p>"

	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		HTML(long).
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}

	if got := built.Content.Headers.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
	if got := built.Content.Headers.Get("Content-Transfer-Encoding"); got != "quoted-printable" {
		t.Fatalf("Content-Transfer-Encoding = %q, want quoted-printable", got)
	}
	assertQuotedPrintableLines(t, built.Content.Body)
	if err := built.Content.Validate(); err != nil {
		t.Errorf("Content.Validate() error = %v, want nil", err)
	}
	if got := decodeQuotedPrintable(t, built.Content.Body); got != long {
		t.Errorf("decoded body = %q, want %q", got, long)
	}
}

func TestMessageBuildQuotedPrintableNonASCII(t *testing.T) {
	long := strings.Repeat("ü", 1200)

	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		Text(long).
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}

	if got := built.Content.Headers.Get("Content-Transfer-Encoding"); got != "quoted-printable" {
		t.Fatalf("Content-Transfer-Encoding = %q, want quoted-printable", got)
	}
	if err := built.Content.Validate(); err != nil {
		t.Errorf("Content.Validate() error = %v, want nil", err)
	}
	if got := decodeQuotedPrintable(t, built.Content.Body); got != long {
		t.Errorf("decoded body = %q, want %q", got, long)
	}
}

func TestMessageBuildQuotedPrintableAlternative(t *testing.T) {
	longText := strings.Repeat("t", 1100)
	longHTML := "<p>" + strings.Repeat("h", 1100) + "</p>"

	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		Text(longText).
		HTML(longHTML).
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(built.Content.Headers.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse Content-Type: %v", err)
	}
	if mediaType != "multipart/alternative" {
		t.Fatalf("media type = %q, want multipart/alternative", mediaType)
	}

	reader := multipart.NewReader(bytes.NewReader(built.Content.Body), params["boundary"])
	want := []struct {
		contentType string
		body        string
	}{
		{"text/plain; charset=utf-8", longText},
		{"text/html; charset=utf-8", longHTML},
	}
	for i, w := range want {
		part, err := reader.NextRawPart()
		if err != nil {
			t.Fatalf("part %d: NextRawPart() error = %v", i, err)
		}
		if got := part.Header.Get("Content-Type"); got != w.contentType {
			t.Errorf("part %d Content-Type = %q, want %q", i, got, w.contentType)
		}
		if got := part.Header.Get("Content-Transfer-Encoding"); got != "quoted-printable" {
			t.Errorf("part %d Content-Transfer-Encoding = %q, want quoted-printable", i, got)
		}
		decoded, err := io.ReadAll(quotedprintable.NewReader(part))
		if err != nil {
			t.Fatalf("part %d decode: %v", i, err)
		}
		if string(decoded) != w.body {
			t.Errorf("part %d decoded body = %q, want %q", i, decoded, w.body)
		}
	}
	if _, err := reader.NextRawPart(); err != io.EOF {
		t.Errorf("NextRawPart() after last part error = %v, want io.EOF", err)
	}
	if err := built.Content.Validate(); err != nil {
		t.Errorf("Content.Validate() error = %v, want nil", err)
	}
}

func TestMessageBuildKeepsShortBodiesUnencoded(t *testing.T) {
	built, err := NewMessage().
		From("a@example.com").
		To("b@example.com").
		Text("short body\r\nsecond line").
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	if got := built.Content.Headers.Get("Content-Transfer-Encoding"); got != "7bit" {
		t.Errorf("Content-Transfer-Encoding = %q, want 7bit", got)
	}
}

func assertQuotedPrintableLines(t *testing.T, body []byte) {
	t.Helper()
	for _, line := range strings.Split(string(body), "\r\n") {
		if len(line) > 76 {
			t.Errorf("encoded line length = %d, want <= 76: %q", len(line), line)
		}
	}
}

func decodeQuotedPrintable(t *testing.T, body []byte) string {
	t.Helper()
	decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
	if err != nil {
		t.Fatalf("decode quoted-printable: %v", err)
	}
	return string(decoded)
}
