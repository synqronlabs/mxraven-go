package mime

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// alternativeWithAttachment ports the ALTERNATIVE_WITH_ATTACHMENT fixture from
// the JavaScript SDK's parse test, using LF line endings to exercise lenient
// parsing.
const alternativeWithAttachment = "Subject: Test message\n" +
	`From: "Alice Example" <alice@example.com>` + "\n" +
	`To: bob@example.com, "Carol" <carol@example.com>` + "\n" +
	"Date: Sun, 13 Sep 2026 14:30:00 +0000\n" +
	"Message-ID: <abc@example.com>\n" +
	"MIME-Version: 1.0\n" +
	`Content-Type: multipart/mixed; boundary="BOUND1"` + "\n" +
	"\n" +
	"--BOUND1\n" +
	`Content-Type: multipart/alternative; boundary="BOUND2"` + "\n" +
	"\n" +
	"--BOUND2\n" +
	"Content-Type: text/plain; charset=utf-8\n" +
	"Content-Transfer-Encoding: quoted-printable\n" +
	"\n" +
	"Hello =E2=9C=93\n" +
	"soft=\n" +
	"break\n" +
	"--BOUND2\n" +
	"Content-Type: text/html; charset=utf-8\n" +
	"Content-Transfer-Encoding: 7bit\n" +
	"\n" +
	"<html><body>Hi</body></html>\n" +
	"--BOUND2--\n" +
	"--BOUND1\n" +
	`Content-Type: application/octet-stream; name="hello.txt"` + "\n" +
	`Content-Disposition: attachment; filename="hello.txt"` + "\n" +
	"Content-Transfer-Encoding: base64\n" +
	"\n" +
	"SGVsbG8gZmlsZSE=\n" +
	"--BOUND1--\n"

func TestParseHeadersAddressesAndDate(t *testing.T) {
	email, err := ParseEmail([]byte(alternativeWithAttachment))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Subject, "Test message"; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if got, want := email.MessageID, "abc@example.com"; got != want {
		t.Errorf("MessageID = %q, want %q", got, want)
	}
	if want := time.Date(2026, 9, 13, 14, 30, 0, 0, time.UTC); !email.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", email.Date, want)
	}
	if got, want := email.From, []Address{{Name: "Alice Example", Address: "alice@example.com"}}; !addressesEqual(got, want) {
		t.Errorf("From = %+v, want %+v", got, want)
	}
	wantTo := []Address{{Address: "bob@example.com"}, {Name: "Carol", Address: "carol@example.com"}}
	if !addressesEqual(email.To, wantTo) {
		t.Errorf("To = %+v, want %+v", email.To, wantTo)
	}
}

func TestParseBodies(t *testing.T) {
	email, err := ParseEmail([]byte(alternativeWithAttachment))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Text, "Hello \u2713\nsoftbreak"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if got, want := email.HTML, "<html><body>Hi</body></html>"; got != want {
		t.Errorf("HTML = %q, want %q", got, want)
	}
}

func TestParseAttachment(t *testing.T) {
	email, err := ParseEmail([]byte(alternativeWithAttachment))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if len(email.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(email.Attachments))
	}
	attachment := email.Attachments[0]
	if got, want := attachment.Filename, "hello.txt"; got != want {
		t.Errorf("Filename = %q, want %q", got, want)
	}
	if got, want := attachment.ContentType, "application/octet-stream"; got != want {
		t.Errorf("ContentType = %q, want %q", got, want)
	}
	if attachment.Inline {
		t.Error("Inline = true, want false")
	}
	if got, want := string(attachment.Content), "Hello file!"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParseEncodedWords(t *testing.T) {
	raw := "Subject: =?UTF-8?B?SGVsbG8gV29ybGQ=?=\r\n" +
		"From: =?UTF-8?Q?Jos=C3=A9?= <jose@example.com>\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"body"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Subject, "Hello World"; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if want := []Address{{Name: "José", Address: "jose@example.com"}}; !addressesEqual(email.From, want) {
		t.Errorf("From = %+v, want %+v", email.From, want)
	}
}

func TestParseSimpleBody(t *testing.T) {
	email, err := ParseEmail([]byte("Subject: Simple\r\nContent-Type: text/plain\r\n\r\njust text"))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Text, "just text"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if len(email.Attachments) != 0 {
		t.Errorf("Attachments = %+v, want none", email.Attachments)
	}
}

func TestParseRFC2231Filename(t *testing.T) {
	raw := "Content-Type: application/octet-stream\r\n" +
		"Content-Disposition: attachment; filename*=UTF-8''%C3%A9.txt\r\n" +
		"\r\n" +
		"data"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Attachments[0].Filename, "é.txt"; got != want {
		t.Errorf("Filename = %q, want %q", got, want)
	}
}

func TestParseRFC2231Continuation(t *testing.T) {
	raw := "Content-Type: application/octet-stream\r\n" +
		"Content-Disposition: attachment; filename*0*=UTF-8''%C3%A9; filename*1*=.txt\r\n" +
		"\r\n" +
		"data"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Attachments[0].Filename, "é.txt"; got != want {
		t.Errorf("Filename = %q, want %q", got, want)
	}
}

func TestParseEmptyMessage(t *testing.T) {
	email, err := ParseEmail(nil)
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if email.Subject != "" || email.Text != "" || len(email.Headers) != 0 || len(email.Attachments) != 0 {
		t.Fatalf("Email = %+v, want empty", email)
	}
}

func TestParseLFOnly(t *testing.T) {
	email, err := ParseEmail([]byte("Subject: LF\n\nbody text"))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Subject, "LF"; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if got, want := email.Text, "body text"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

func TestParseBase64WithWhitespace(t *testing.T) {
	raw := "Content-Type: application/octet-stream\r\n" +
		`Content-Disposition: attachment; filename="a.bin"` + "\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"SGVs\r\nbG8g\r\nZmlsZSE="
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := string(email.Attachments[0].Content), "Hello file!"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParseInlinePart(t *testing.T) {
	raw := `Content-Type: multipart/related; boundary="R"` + "\r\n" +
		"\r\n" +
		"--R\r\n" +
		"Content-Type: text/html\r\n" +
		"\r\n" +
		`<img src="cid:img1">` + "\r\n" +
		"--R\r\n" +
		"Content-Type: image/png\r\n" +
		"Content-ID: <img1>\r\n" +
		`Content-Disposition: inline; filename="logo.png"` + "\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"AQID\r\n" +
		"--R--"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.HTML, `<img src="cid:img1">`; got != want {
		t.Errorf("HTML = %q, want %q", got, want)
	}
	if len(email.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(email.Attachments))
	}
	attachment := email.Attachments[0]
	if got, want := attachment.Filename, "logo.png"; got != want {
		t.Errorf("Filename = %q, want %q", got, want)
	}
	if got, want := attachment.ContentType, "image/png"; got != want {
		t.Errorf("ContentType = %q, want %q", got, want)
	}
	if got, want := attachment.ContentID, "img1"; got != want {
		t.Errorf("ContentID = %q, want %q", got, want)
	}
	if !attachment.Inline {
		t.Error("Inline = false, want true")
	}
	if !bytes.Equal(attachment.Content, []byte{1, 2, 3}) {
		t.Errorf("Content = %v, want [1 2 3]", attachment.Content)
	}
}

func TestParseInlinePartInMixed(t *testing.T) {
	// The Go submission builder wraps a body and its inline attachments in
	// multipart/mixed, so an inline image is a sibling of the multipart/alternative
	// body rather than a multipart/related child.
	raw := `Content-Type: multipart/mixed; boundary="B"` + "\r\n" +
		"\r\n" +
		"--B\r\n" +
		`Content-Type: multipart/alternative; boundary="A"` + "\r\n" +
		"\r\n" +
		"--A\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"text body\r\n" +
		"--A\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		`<img src="cid:logo">` + "\r\n" +
		"--A--\r\n" +
		"--B\r\n" +
		"Content-Type: image/png\r\n" +
		`Content-Disposition: inline; filename="logo.png"` + "\r\n" +
		"Content-ID: <logo>\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"aW1n\r\n" +
		"--B--"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Text, "text body"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if got, want := email.HTML, `<img src="cid:logo">`; got != want {
		t.Errorf("HTML = %q, want %q", got, want)
	}
	if len(email.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(email.Attachments))
	}
	attachment := email.Attachments[0]
	if got, want := attachment.ContentID, "logo"; got != want {
		t.Errorf("ContentID = %q, want %q", got, want)
	}
	if !attachment.Inline {
		t.Error("Inline = false, want true")
	}
	if got, want := string(attachment.Content), "img"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParseQuotedCommasInDisplayNames(t *testing.T) {
	raw := "To: \"Doe, John\" <john@example.com>, jane@example.com\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"body"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	want := []Address{{Name: "Doe, John", Address: "john@example.com"}, {Address: "jane@example.com"}}
	if !addressesEqual(email.To, want) {
		t.Errorf("To = %+v, want %+v", email.To, want)
	}
}

func TestParseAddressGroups(t *testing.T) {
	raw := "To: Friends: a@example.com, \"Doe, John\" <john@example.com>;\r\n" +
		"Cc: undisclosed-recipients:;\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"body"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	wantTo := []Address{
		{Address: "a@example.com"},
		{Name: "Doe, John", Address: "john@example.com"},
	}
	if !addressesEqual(email.To, wantTo) {
		t.Errorf("To = %+v, want %+v", email.To, wantTo)
	}
	if len(email.Cc) != 0 {
		t.Errorf("Cc = %+v, want no addresses", email.Cc)
	}
}

func TestParseObsoleteAddressListFallsBack(t *testing.T) {
	raw := "To: a@example.com,, b@example.com,\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"body"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	want := []Address{{Address: "a@example.com"}, {Address: "b@example.com"}}
	if !addressesEqual(email.To, want) {
		t.Errorf("To = %+v, want %+v", email.To, want)
	}
}

func TestParseMessageRFC822AsAttachment(t *testing.T) {
	raw := "Content-Type: message/rfc822\r\n" +
		"\r\n" +
		"From: inner@example.com\r\n" +
		"Subject: Inner\r\n" +
		"\r\n" +
		"inner body"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if len(email.Attachments) != 1 {
		t.Fatalf("len(Attachments) = %d, want 1", len(email.Attachments))
	}
	if got, want := email.Attachments[0].ContentType, "message/rfc822"; got != want {
		t.Errorf("ContentType = %q, want %q", got, want)
	}
	if !strings.Contains(string(email.Attachments[0].Content), "inner body") {
		t.Errorf("Content = %q, want it to contain %q", email.Attachments[0].Content, "inner body")
	}
}

func TestParseUnfoldsHeaderContinuations(t *testing.T) {
	email, err := ParseEmail([]byte("Subject: Hello\r\n World\r\n\r\nbody"))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Subject, "Hello World"; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
}

func TestParseDeclaredCharset(t *testing.T) {
	raw := append([]byte("Content-Type: text/plain; charset=windows-1252\r\n\r\n"), 0x93)
	email, err := ParseEmail(raw)
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Text, "\u201c"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

func TestParseUnknownCharsetFallsBackToUTF8(t *testing.T) {
	raw := "Content-Type: text/plain; charset=x-unknown\r\n\r\nhéllo"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Text, "héllo"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

func TestParseLatin1Fallback(t *testing.T) {
	raw := append([]byte("Content-Type: text/plain; charset=us-ascii\r\n\r\n"), 0xe9)
	email, err := ParseEmail(raw)
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if got, want := email.Text, "é"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

func TestParseRejectsDeepNesting(t *testing.T) {
	message := "Content-Type: text/plain\r\n\r\nleaf"
	for depth := 0; depth < 25; depth++ {
		boundary := fmt.Sprintf("D%d", depth)
		message = fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n\r\n--%s\r\n%s\r\n--%s--", boundary, boundary, message, boundary)
	}
	_, err := ParseEmail([]byte(message))
	if !errors.Is(err, ErrTooDeep) {
		t.Fatalf("ParseEmail() error = %v, want ErrTooDeep", err)
	}
}

func TestParseRejectsTooManyParts(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("Content-Type: multipart/mixed; boundary=B\r\n\r\n")
	for index := 0; index < maxParts+1; index++ {
		builder.WriteString("--B\r\nContent-Type: text/plain\r\n\r\nx\r\n")
	}
	builder.WriteString("--B--\r\n")
	_, err := ParseEmail([]byte(builder.String()))
	if !errors.Is(err, ErrTooManyParts) {
		t.Fatalf("ParseEmail() error = %v, want ErrTooManyParts", err)
	}
}

func TestParseAttachmentContentDoesNotAliasInput(t *testing.T) {
	raw := "Content-Type: application/octet-stream\r\n" +
		`Content-Disposition: attachment; filename="a.bin"` + "\r\n" +
		"\r\n" +
		"payload"
	data := []byte(raw)
	email, err := ParseEmail(data)
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	for index := range data {
		data[index] = 0
	}
	if got, want := string(email.Attachments[0].Content), "payload"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
}

func TestParseContentIDsMakeInline(t *testing.T) {
	raw := "Content-Type: image/png\r\n" +
		"Content-ID: <img-1>\r\n" +
		"\r\n" +
		"png"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if len(email.Attachments) != 1 || !email.Attachments[0].Inline {
		t.Fatalf("Attachments = %+v, want one inline attachment", email.Attachments)
	}
	if got, want := email.Attachments[0].ContentID, "img-1"; got != want {
		t.Errorf("ContentID = %q, want %q", got, want)
	}
}

func TestParseAttachmentDispositionIsNotInline(t *testing.T) {
	raw := "Content-Type: image/png\r\n" +
		"Content-ID: <img-1>\r\n" +
		`Content-Disposition: attachment; filename="logo.png"` + "\r\n" +
		"\r\n" +
		"png"
	email, err := ParseEmail([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if len(email.Attachments) != 1 || email.Attachments[0].Inline {
		t.Fatalf("Attachments = %+v, want one non-inline attachment", email.Attachments)
	}
}

func TestParseMalformedAddressFallsBack(t *testing.T) {
	email, err := ParseEmail([]byte("To: not an address, \"Doe, John\" <john@example.com>\r\n\r\nbody"))
	if err != nil {
		t.Fatalf("ParseEmail() error = %v", err)
	}
	if len(email.To) != 2 {
		t.Fatalf("To = %+v, want two addresses", email.To)
	}
	if got, want := email.To[1].Address, "john@example.com"; got != want {
		t.Errorf("To[1].Address = %q, want %q", got, want)
	}
}

func TestParseMediaTypeFallback(t *testing.T) {
	primary, subtype, _ := parseMediaType("TEXT/PLAIN; charset=utf-8")
	if primary != "text" || subtype != "plain" {
		t.Errorf("parseMediaType() = %q/%q, want text/plain", primary, subtype)
	}
	primary, subtype, _ = parseMediaType("not a media type")
	if primary != "not a media type" || subtype != "" {
		t.Errorf("parseMediaType() = %q/%q, want the raw token", primary, subtype)
	}
}

func addressesEqual(got, want []Address) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
