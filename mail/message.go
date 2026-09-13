package mail

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
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
		builder.HTMLBody(m.html)
	case m.text != "":
		builder.TextBody(m.text)
	}
	for _, attachment := range m.attachments {
		if attachment.Inline {
			builder.AttachInline(attachment.Filename, attachment.ContentID, attachment.Data, attachment.ContentType)
			continue
		}
		builder.AttachFile(attachment.Filename, attachment.Data, attachment.ContentType)
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
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", part.contentType)
		header.Set("Content-Transfer-Encoding", string(encoding))
		partWriter, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", "", fmt.Errorf("mail: build alternative part: %w", err)
		}
		if _, err := io.WriteString(partWriter, normalizeCRLF(part.body)); err != nil {
			return nil, "", "", fmt.Errorf("mail: write alternative part: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", "", fmt.Errorf("mail: close alternative body: %w", err)
	}
	return buf.Bytes(), "multipart/alternative; boundary=" + writer.Boundary(), encoding, nil
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
