package mail

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"
	"time"
	"unicode/utf8"

	ravenmail "github.com/synqronlabs/raven/mail"
)

// Message is a mutable builder for an email message.
//
// Chained methods return the receiver, so a message is normally composed in a
// single expression. A Message is not safe for concurrent use. It can be sent
// repeatedly; each [Client.Send] serializes the current state.
type Message struct {
	from        string
	nullSender  bool
	sender      string
	replyTo     string
	to          []string
	cc          []string
	bcc         []string
	subject     string
	text        string
	html        string
	headers     []Header
	attachments []Attachment
	messageID   string
	inReplyTo   string
	references  []string
	date        time.Time
	deliveryBy  *DeliveryBy
	requireTLS  bool
}

// Header is a custom message header.
type Header struct {
	// Name is the header field name.
	Name string
	// Value is the header field value. It must not contain line breaks.
	Value string
}

// Attachment is a message attachment.
type Attachment struct {
	// Filename is the attachment filename. It may be empty.
	Filename string
	// ContentType is the media type. When empty, "application/octet-stream"
	// is used.
	ContentType string
	// Data is the raw attachment content. The caller retains ownership; the
	// data is not mutated.
	Data []byte
	// Inline marks the attachment for inline display, for example an image
	// referenced by ContentID from an HTML body.
	Inline bool
	// ContentID is the inline content identifier. It is ignored when Inline
	// is false.
	ContentID string
}

// NewMessage returns an empty message builder.
func NewMessage() *Message {
	return &Message{}
}

// From sets the envelope sender and the From header. The address may include a
// display name, for example "Acme <noreply@acme.example>".
func (m *Message) From(address string) *Message {
	m.from = address
	return m
}

// NullSender uses a null reverse-path while keeping the From header set by
// [Message.From]. It is intended for bounce and other auto-generated messages.
//
// A From header is still required for a valid message.
func (m *Message) NullSender() *Message {
	m.nullSender = true
	return m
}

// Sender sets the Sender header. It is required when the From header contains
// more than one mailbox.
func (m *Message) Sender(address string) *Message {
	m.sender = address
	return m
}

// ReplyTo sets the Reply-To header.
func (m *Message) ReplyTo(address string) *Message {
	m.replyTo = address
	return m
}

// To adds envelope and To-header recipients.
func (m *Message) To(addresses ...string) *Message {
	m.to = append(m.to, addresses...)
	return m
}

// Cc adds envelope and Cc-header recipients.
func (m *Message) Cc(addresses ...string) *Message {
	m.cc = append(m.cc, addresses...)
	return m
}

// Bcc adds envelope recipients without a visible Bcc header.
func (m *Message) Bcc(addresses ...string) *Message {
	m.bcc = append(m.bcc, addresses...)
	return m
}

// Subject sets the Subject header. Non-ASCII subjects are encoded.
func (m *Message) Subject(subject string) *Message {
	m.subject = subject
	return m
}

// MessageID sets the Message-ID header. The value is wrapped in angle brackets
// when it is not already.
func (m *Message) MessageID(id string) *Message {
	m.messageID = id
	return m
}

// InReplyTo sets the In-Reply-To header for threading.
func (m *Message) InReplyTo(id string) *Message {
	m.inReplyTo = id
	return m
}

// References sets the References header for threading.
func (m *Message) References(ids ...string) *Message {
	m.references = append(m.references, ids...)
	return m
}

// Date sets the Date header. When unset, the time of sending is used.
func (m *Message) Date(t time.Time) *Message {
	m.date = t
	return m
}

// DeliveryBy requests delivery within a bounded window using the DELIVERBY
// extension (RFC 2852). The submission server must advertise DELIVERBY or the
// send fails with [ErrDeliveryByUnsupported].
func (m *Message) DeliveryBy(by DeliveryBy) *Message {
	m.deliveryBy = &by
	return m
}

// RequireTLS marks the message as requiring TLS on every delivery hop using the
// REQUIRETLS extension (RFC 8689). A compliant receiving server refuses to
// deliver the message over a non-TLS connection, and the submission server must
// advertise REQUIRETLS or the send is rejected.
func (m *Message) RequireTLS() *Message {
	m.requireTLS = true
	return m
}

// Text sets the plain-text body. When both [Message.Text] and [Message.HTML]
// are set, the message is sent as multipart/alternative.
func (m *Message) Text(body string) *Message {
	m.text = body
	return m
}

// HTML sets the HTML body. When both [Message.Text] and [Message.HTML] are
// set, the message is sent as multipart/alternative.
func (m *Message) HTML(body string) *Message {
	m.html = body
	return m
}

// Header appends a custom header.
func (m *Message) Header(name, value string) *Message {
	m.headers = append(m.headers, Header{Name: name, Value: value})
	return m
}

// Attach appends an attachment.
//
// The attachment data is retained by reference until the message is sent.
// Callers must not mutate it in the meantime.
func (m *Message) Attach(attachment Attachment) *Message {
	m.attachments = append(m.attachments, attachment)
	return m
}

// AttachFile appends a file attachment with an "application/octet-stream"
// content type. Use [Message.Attach] to set a different content type.
func (m *Message) AttachFile(filename string, data []byte) *Message {
	return m.Attach(Attachment{Filename: filename, Data: data})
}

// AttachInline appends an inline attachment referenced by contentID, for
// example from an HTML body using "cid:".
func (m *Message) AttachInline(filename, contentID string, data []byte) *Message {
	return m.Attach(Attachment{
		Filename:  filename,
		Data:      data,
		Inline:    true,
		ContentID: contentID,
	})
}

// build serializes the message into a raven mail.
func (m *Message) build() (*ravenmail.Mail, error) {
	builder := ravenmail.NewMailBuilder()
	if from := strings.TrimSpace(m.from); from != "" {
		builder.From(from)
	}
	if m.nullSender {
		builder.NullSender()
	}
	if m.sender != "" {
		builder.Sender(m.sender)
	}
	if m.replyTo != "" {
		builder.ReplyTo(m.replyTo)
	}
	if len(m.to) > 0 {
		builder.To(m.to...)
	}
	if len(m.cc) > 0 {
		builder.Cc(m.cc...)
	}
	if len(m.bcc) > 0 {
		builder.Bcc(m.bcc...)
	}
	if m.subject != "" {
		builder.Subject(m.subject)
	}
	if m.messageID != "" {
		builder.MessageID(m.messageID)
	}
	if m.inReplyTo != "" {
		builder.InReplyTo(m.inReplyTo)
	}
	if len(m.references) > 0 {
		builder.References(m.references...)
	}
	if !m.date.IsZero() {
		builder.Date(m.date)
	}
	for _, header := range m.headers {
		builder.Header(header.Name, header.Value)
	}
	switch {
	case m.text != "" && m.html != "":
		body, contentType, encoding, err := buildAlternative(m.text, m.html)
		if err != nil {
			return nil, err
		}
		builder.Body(body, contentType, encoding)
	case m.html != "":
		if needsQuotedPrintable(m.html) {
			if err := setQuotedPrintableBody(builder, m.html, "text/html; charset=utf-8"); err != nil {
				return nil, err
			}
		} else {
			builder.HTMLBody(m.html)
		}
	case m.text != "":
		if needsQuotedPrintable(m.text) {
			if err := setQuotedPrintableBody(builder, m.text, "text/plain; charset=utf-8"); err != nil {
				return nil, err
			}
		} else {
			builder.TextBody(m.text)
		}
	}
	for _, attachment := range m.attachments {
		if attachment.Inline {
			builder.AttachInline(attachment.Filename, attachment.ContentID, attachment.Data, attachment.ContentType)
			continue
		}
		builder.AttachFile(attachment.Filename, attachment.Data, attachment.ContentType)
	}
	if m.deliveryBy != nil {
		deliveryBy, err := m.deliveryBy.raven()
		if err != nil {
			return nil, err
		}
		builder.DeliveryBy(deliveryBy.Seconds, deliveryBy.Mode, deliveryBy.Trace)
	}
	if m.requireTLS {
		builder.RequireTLS()
	}

	built, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("mail: build message: %w", err)
	}
	return built, nil
}

// buildAlternative renders plain-text and HTML bodies as a
// multipart/alternative body.
func buildAlternative(text, html string) ([]byte, string, ravenmail.ContentTransferEncoding, error) {
	encoding := ravenmail.Encoding7Bit
	if containsNonASCII(text) || containsNonASCII(html) {
		encoding = ravenmail.Encoding8Bit
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	parts := []struct {
		contentType string
		body        string
	}{
		{"text/plain; charset=utf-8", text},
		{"text/html; charset=utf-8", html},
	}
	for _, part := range parts {
		body, partEncoding, err := encodeAlternativePart(part.body, encoding)
		if err != nil {
			return nil, "", "", err
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", part.contentType)
		header.Set("Content-Transfer-Encoding", string(partEncoding))
		partWriter, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", "", fmt.Errorf("mail: build alternative part: %w", err)
		}
		if _, err := partWriter.Write(body); err != nil {
			return nil, "", "", fmt.Errorf("mail: write alternative part: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", "", fmt.Errorf("mail: close alternative body: %w", err)
	}
	return buf.Bytes(), "multipart/alternative; boundary=" + writer.Boundary(), encoding, nil
}

// setQuotedPrintableBody replaces a single text or HTML body with its
// quoted-printable encoded form, which satisfies the RFC 5322 line-length limit
// without altering the decoded content.
func setQuotedPrintableBody(builder *ravenmail.MailBuilder, body, contentType string) error {
	encoded, err := encodeQuotedPrintable(body)
	if err != nil {
		return err
	}
	builder.Body(encoded, contentType, ravenmail.EncodingQuotedPrintable)
	return nil
}

// encodeAlternativePart returns the wire bytes and transfer encoding for one
// alternative part. Parts with a line longer than the RFC 5322 limit are
// quoted-printable encoded; the rest keep the shared base encoding derived from
// the bodies' character set.
func encodeAlternativePart(body string, base ravenmail.ContentTransferEncoding) ([]byte, ravenmail.ContentTransferEncoding, error) {
	if !needsQuotedPrintable(body) {
		return []byte(normalizeCRLF(body)), base, nil
	}
	encoded, err := encodeQuotedPrintable(body)
	if err != nil {
		return nil, "", err
	}
	return encoded, ravenmail.EncodingQuotedPrintable, nil
}

// needsQuotedPrintable reports whether body has a line longer than the RFC 5322
// limit and therefore must be transfer-encoded before submission.
func needsQuotedPrintable(body string) bool {
	return longestLine(normalizeCRLF(body)) > ravenmail.MaxLineLength
}

// longestLine returns the byte length of the longest line in s, treating LF,
// CRLF, and a bare CR as line separators.
func longestLine(s string) int {
	longest, current := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n':
			if current > longest {
				longest = current
			}
			current = 0
		case '\r':
			if current > longest {
				longest = current
			}
			current = 0
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
		default:
			current++
		}
	}
	if current > longest {
		longest = current
	}
	return longest
}

// encodeQuotedPrintable renders body in quoted-printable form. Lines are at
// most 76 bytes, so the result always satisfies the RFC 5322 line-length limit,
// and decoding restores the original body bytes.
func encodeQuotedPrintable(body string) ([]byte, error) {
	var buf bytes.Buffer
	writer := quotedprintable.NewWriter(&buf)
	if _, err := io.WriteString(writer, normalizeCRLF(body)); err != nil {
		return nil, fmt.Errorf("mail: encode quoted-printable: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("mail: encode quoted-printable: %w", err)
	}
	return buf.Bytes(), nil
}

// containsNonASCII reports whether s contains a non-ASCII byte.
func containsNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return true
		}
	}
	return false
}

// normalizeCRLF converts LF and bare CR line endings to CRLF.
func normalizeCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}
