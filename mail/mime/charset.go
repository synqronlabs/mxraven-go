package mime

import (
	"fmt"
	"io"
	stdmime "mime"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/ianaindex"
)

// wordDecoder decodes RFC 2047 encoded words, including non-UTF-8 charsets.
var wordDecoder = stdmime.WordDecoder{CharsetReader: charsetReader}

// charsetReader returns a reader that decodes charset, for [stdmime.WordDecoder].
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	label := strings.ToLower(strings.TrimSpace(charset))
	if label == "" || label == "us-ascii" || label == "ascii" {
		return input, nil
	}
	encoding, err := ianaindex.MIME.Encoding(label)
	if err != nil || encoding == nil {
		return nil, fmt.Errorf("mime: unsupported charset %q", charset)
	}
	return encoding.NewDecoder().Reader(input), nil
}

// decodeHeaderText decodes a header value: raw 8-bit content that is not valid
// UTF-8 is treated as Latin-1, then RFC 2047 encoded words are decoded.
func decodeHeaderText(value string) string {
	text := value
	if !utf8.ValidString(text) {
		text = latin1String(text)
	}
	decoded, err := wordDecoder.DecodeHeader(text)
	if err != nil {
		return text
	}
	return decoded
}

// decodeBytes decodes message bytes from a charset label, falling back to UTF-8
// and then Windows-1252 for unknown or undecodable content.
func decodeBytes(data []byte, charset string) string {
	label := strings.ToLower(strings.TrimSpace(charset))
	switch label {
	case "", "us-ascii", "ascii", "unknown-8bit":
		// Fall through to the heuristic below.
	default:
		if encoding, err := ianaindex.MIME.Encoding(label); err == nil && encoding != nil {
			if decoded, err := encoding.NewDecoder().Bytes(data); err == nil {
				return string(decoded)
			}
		}
	}
	if utf8.Valid(data) {
		return string(data)
	}
	decoded, _ := charmap.Windows1252.NewDecoder().Bytes(data)
	return string(decoded)
}

// latin1String maps each byte to the code point with the same value, the way a
// Latin-1 decoder would.
func latin1String(value string) string {
	runes := make([]rune, 0, len(value))
	for index := 0; index < len(value); index++ {
		runes = append(runes, rune(value[index]))
	}
	return string(runes)
}
