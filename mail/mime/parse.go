package mime

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	stdmime "mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	ravenmail "github.com/synqronlabs/raven/mail"
)

// Parsing limits protect against unbounded input from untrusted messages.
const (
	maxDepth = 20
	maxParts = 2000
)

// ErrTooDeep reports a message whose MIME nesting exceeds the parser limit.
var ErrTooDeep = errors.New("mime: message nesting is too deep")

// ErrTooManyParts reports a message whose MIME entity count exceeds the parser
// limit.
var ErrTooManyParts = errors.New("mime: too many MIME parts")

// Header is one header field from a parsed message.
type Header struct {
	// Name is the field name as it appeared in the message.
	Name string
	// Value is the unfolded field value. Encoded words are not decoded here;
	// use [Email.Subject] or the address fields for decoded text.
	Value string
}

// Address is one mailbox from a parsed address header.
type Address struct {
	// Name is the display name, with RFC 2047 encoded words decoded, when one
	// was present.
	Name string
	// Address is the bare local@domain form.
	Address string
}

// Attachment is one non-body MIME part.
type Attachment struct {
	// Filename is the decoded filename, when one was present.
	Filename string
	// ContentType is the media type, for example "application/pdf".
	ContentType string
	// ContentID is the Content-ID value without angle brackets, for inline
	// parts.
	ContentID string
	// Inline reports whether the part is marked inline or carries a Content-ID.
	Inline bool
	// Content is the decoded attachment bytes. It is a fresh copy that the
	// caller owns and may mutate.
	Content []byte
}

// Email is a parsed inbound RFC 5322 / MIME message.
//
// Zero values mean the corresponding header or body was absent: an empty
// Subject, MessageID, Text, or HTML, or a zero Date. From, To, Cc, Bcc, and
// ReplyTo are nil when the header was absent.
type Email struct {
	// Headers holds every header field, in order, with continuation lines
	// unfolded. Values are not decoded.
	Headers []Header
	// Subject is the decoded Subject header, when present.
	Subject string
	// From holds the From addresses.
	From []Address
	// To holds the To addresses.
	To []Address
	// Cc holds the Cc addresses.
	Cc []Address
	// Bcc holds the Bcc addresses.
	Bcc []Address
	// ReplyTo holds the Reply-To addresses.
	ReplyTo []Address
	// MessageID is the Message-ID without angle brackets, when present.
	MessageID string
	// Date is the parsed Date header, when present and parseable.
	Date time.Time
	// Text is the decoded first text/plain body, when the message has one.
	Text string
	// HTML is the decoded first text/html body, when the message has one.
	HTML string
	// Attachments holds every non-body part, with transfer encoding and
	// filename decoding applied. message/rfc822 parts appear here rather than
	// being parsed recursively.
	Attachments []Attachment
}

// ParseEmail parses an inbound RFC 5322 / MIME message.
//
// It never fails on malformed content; it extracts what it can. The returned
// message contains fresh copies of attachment content and does not retain data.
func ParseEmail(data []byte) (*Email, error) {
	var parser parser
	email := &Email{}
	if len(data) > 0 {
		headers, body := splitEntity(data)
		email.Headers = headers
		if err := parser.walk(headers, body, 0); err != nil {
			return nil, err
		}
	}

	email.Subject = decodeHeaderText(lookup(email.Headers, "subject"))
	email.From = parseAddressList(lookup(email.Headers, "from"))
	email.To = parseAddressList(lookup(email.Headers, "to"))
	email.Cc = parseAddressList(lookup(email.Headers, "cc"))
	email.Bcc = parseAddressList(lookup(email.Headers, "bcc"))
	email.ReplyTo = parseAddressList(lookup(email.Headers, "reply-to"))
	email.MessageID = messageID(lookup(email.Headers, "message-id"))
	email.Date = parseDate(lookup(email.Headers, "date"))
	email.Text = parser.text
	email.HTML = parser.html
	email.Attachments = parser.attachments
	return email, nil
}

// parser accumulates state while walking a message tree.
type parser struct {
	text        string
	html        string
	hasText     bool
	hasHTML     bool
	attachments []Attachment
	parts       int
}

// headerGetter is the minimal header view the walk needs. It is satisfied by
// the parsed [Header] slice and by [net/textproto.MIMEHeader].
type headerGetter interface {
	Get(name string) string
}

// headerList is an ordered header list that implements headerGetter.
type headerList []Header

// Get returns the first value for name, compared case-insensitively.
func (h headerList) Get(name string) string {
	return lookup(h, name)
}

// lookup returns the first value for name, compared case-insensitively.
func lookup(headers []Header, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

// entity is a header block and its raw body.
type entity struct {
	headers headerGetter
	body    []byte
}

// walk decodes one entity and recurses into multipart children.
func (p *parser) walk(headers headerGetter, body []byte, depth int) error {
	if depth > maxDepth {
		return ErrTooDeep
	}
	p.parts++
	if p.parts > maxParts {
		return ErrTooManyParts
	}

	contentType, subtype, contentTypeParams := parseMediaType(headers.Get("Content-Type"))
	if contentType == "multipart" && subtype != "" {
		if boundary := contentTypeParams["boundary"]; boundary != "" {
			for _, part := range readMultipart(body, boundary) {
				if err := p.walk(part.headers, part.body, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
	}

	encoding := headers.Get("Content-Transfer-Encoding")
	dispositionType, _, dispositionParams := parseMediaType(headers.Get("Content-Disposition"))
	placement := strings.ToLower(dispositionParams["disposition"])
	if placement == "" {
		placement = strings.ToLower(dispositionType)
	}
	filename := dispositionParams["filename"]
	if filename == "" {
		filename = contentTypeParams["name"]
	}
	contentID := strings.TrimSpace(headers.Get("Content-ID"))
	contentID = strings.TrimPrefix(contentID, "<")
	contentID = strings.TrimSuffix(contentID, ">")
	hasFilename := filename != ""
	isAttachment := placement == "attachment" || hasFilename || contentID != ""

	decoded := decodeTransferEncoding(body, encoding)

	if !isAttachment && contentType == "text" && subtype == "plain" {
		if !p.hasText {
			p.hasText = true
			p.text = decodeBytes(decoded, contentTypeParams["charset"])
		}
		return nil
	}
	if !isAttachment && contentType == "text" && subtype == "html" {
		if !p.hasHTML {
			p.hasHTML = true
			p.html = decodeBytes(decoded, contentTypeParams["charset"])
		}
		return nil
	}

	attachment := Attachment{
		ContentType: contentType,
		ContentID:   contentID,
		Inline:      placement == "inline" || (placement != "attachment" && contentID != ""),
		Content:     append([]byte(nil), decoded...),
	}
	if subtype != "" {
		attachment.ContentType = contentType + "/" + subtype
	}
	if hasFilename {
		attachment.Filename = decodeHeaderText(filename)
	}
	p.attachments = append(p.attachments, attachment)
	return nil
}

// readMultipart returns the parts of a multipart body. Malformed input yields
// the parts that were readable rather than an error.
func readMultipart(body []byte, boundary string) []entity {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var parts []entity
	for {
		part, err := reader.NextPart()
		if err != nil {
			return parts
		}
		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return parts
		}
		parts = append(parts, entity{headers: part.Header, body: data})
	}
}

// parseMediaType parses a Content-Type or Content-Disposition value. An absent
// or malformed value degrades to the first token with no parameters.
func parseMediaType(value string) (primary, subtype string, params map[string]string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "text", "plain", nil
	}
	mediaType, params, err := stdmime.ParseMediaType(value)
	if err != nil {
		mediaType = value
		if index := strings.IndexByte(mediaType, ';'); index >= 0 {
			mediaType = mediaType[:index]
		}
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
		params = nil
	}
	primary, subtype, _ = strings.Cut(mediaType, "/")
	return primary, subtype, params
}

// splitEntity splits a message into its header block and body.
func splitEntity(data []byte) (headerList, []byte) {
	headerEnd, bodyStart := findHeaderEnd(data)
	headers := parseHeaders(data[:headerEnd])
	if bodyStart > len(data) {
		bodyStart = len(data)
	}
	return headers, data[bodyStart:]
}

// findHeaderEnd returns the offset just before the line terminator that ends
// the header block and the offset where the body starts. When there is no blank
// line the message has no body.
func findHeaderEnd(data []byte) (headerEnd, bodyStart int) {
	for index := 0; index < len(data); index++ {
		lineEnd := data[index]
		if lineEnd != '\n' && lineEnd != '\r' {
			continue
		}
		next := index + 1
		if lineEnd == '\r' && next < len(data) && data[next] == '\n' {
			next++
		}
		if next >= len(data) {
			continue
		}
		switch data[next] {
		case '\r':
			extra := 0
			if next+1 < len(data) && data[next+1] == '\n' {
				extra = 1
			}
			return index, next + 1 + extra
		case '\n':
			return index, next + 1
		}
	}
	return len(data), len(data)
}

// parseHeaders parses and unfolds a header block, preserving field order.
func parseHeaders(block []byte) headerList {
	headers := make(headerList, 0, 16)
	var current *Header
	for _, line := range splitLines(string(block)) {
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && current != nil {
			current.Value += line
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			current = nil
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if name == "" {
			current = nil
			continue
		}
		headers = append(headers, Header{Name: name, Value: trimHeaderValue(line[colon+1:])})
		current = &headers[len(headers)-1]
	}
	return headers
}

// splitLines splits on CRLF, LF, or a bare CR.
func splitLines(value string) []string {
	if !strings.ContainsAny(value, "\r\n") {
		return []string{value}
	}
	var lines []string
	start := 0
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\n':
			lines = append(lines, value[start:index])
			start = index + 1
		case '\r':
			lines = append(lines, value[start:index])
			if index+1 < len(value) && value[index+1] == '\n' {
				index++
			}
			start = index + 1
		}
	}
	if start < len(value) {
		lines = append(lines, value[start:])
	}
	return lines
}

// trimHeaderValue trims leading and trailing spaces and tabs.
func trimHeaderValue(value string) string {
	return strings.Trim(value, " \t")
}

// decodeTransferEncoding decodes base64 and quoted-printable bodies. Unknown
// encodings and decode failures pass the bytes through unchanged.
func decodeTransferEncoding(data []byte, encoding string) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return decodeBase64(data)
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data)))
		if err != nil {
			return data
		}
		return decoded
	default:
		return data
	}
}

// decodeBase64 decodes Base64, ignoring whitespace and other non-alphabet
// bytes. Decoding is best effort: a malformed tail yields what was decoded.
func decodeBase64(data []byte) []byte {
	filtered := make([]byte, 0, len(data))
	for _, b := range data {
		if isBase64Byte(b) {
			filtered = append(filtered, b)
		}
	}
	switch len(filtered) % 4 {
	case 2:
		filtered = append(filtered, '=', '=')
	case 3:
		filtered = append(filtered, '=')
	}
	decoded, _ := base64.StdEncoding.DecodeString(string(filtered))
	return decoded
}

// isBase64Byte reports whether b belongs to the standard Base64 alphabet.
func isBase64Byte(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9':
		return true
	case b == '+', b == '/', b == '=':
		return true
	default:
		return false
	}
}

// messageID reads a Message-ID, stripping surrounding angle brackets.
func messageID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	return value
}

// parseDate parses a Date header, returning the zero time when absent or
// unparseable.
func parseDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := mail.ParseDate(value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// parseAddressList parses an address header into its mailboxes. The whole
// header is parsed first so groups, comments, and obsolete list syntax are
// handled consistently. A value the parser rejects falls back to parsing each
// top-level comma-separated piece on its own, so one malformed entry does not
// discard the rest of the header.
func parseAddressList(value string) []Address {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if list, err := ravenmail.ParseAddressList(value); err == nil {
		addresses := make([]Address, 0, len(list))
		for _, mailbox := range list {
			addresses = append(addresses, Address{Name: mailbox.DisplayName, Address: mailbox.String()})
		}
		return addresses
	}

	var addresses []Address
	for _, piece := range splitAddressList(value) {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			continue
		}
		addresses = append(addresses, parseOneAddress(piece))
	}
	return addresses
}

// parseOneAddress parses a single mailbox, falling back to the raw entry.
func parseOneAddress(piece string) Address {
	if parsed, err := ravenmail.ParseAddress(piece); err == nil {
		return Address{Name: parsed.DisplayName, Address: parsed.String()}
	}
	if open := strings.LastIndexByte(piece, '<'); open >= 0 {
		if close := strings.LastIndexByte(piece, '>'); close > open {
			return Address{Address: strings.TrimSpace(piece[open+1 : close])}
		}
	}
	return Address{Address: piece}
}

// splitAddressList splits an address list on top-level commas, ignoring commas
// inside quoted strings, angle brackets, and comments.
func splitAddressList(value string) []string {
	var pieces []string
	var current strings.Builder
	var quoted bool
	var escaped bool
	var angleDepth, commentDepth int
	for index := 0; index < len(value); index++ {
		char := value[index]
		switch {
		case escaped:
			current.WriteByte(char)
			escaped = false
		case char == '\\':
			current.WriteByte(char)
			escaped = true
		case char == '"':
			quoted = !quoted
			current.WriteByte(char)
		case !quoted && char == '<':
			angleDepth++
			current.WriteByte(char)
		case !quoted && char == '>':
			if angleDepth > 0 {
				angleDepth--
			}
			current.WriteByte(char)
		case !quoted && char == '(':
			commentDepth++
			current.WriteByte(char)
		case !quoted && char == ')':
			if commentDepth > 0 {
				commentDepth--
			}
			current.WriteByte(char)
		case !quoted && char == ',' && angleDepth == 0 && commentDepth == 0:
			pieces = append(pieces, current.String())
			current.Reset()
		default:
			current.WriteByte(char)
		}
	}
	pieces = append(pieces, current.String())
	return pieces
}
